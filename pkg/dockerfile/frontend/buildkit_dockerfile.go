package frontend

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"

	"github.com/werf/werf/pkg/dockerfile"
	dockerfile_instruction "github.com/werf/werf/pkg/dockerfile/instruction"
)

func ParseDockerfileWithBuildkit(dockerfileBytes []byte, opts dockerfile.DockerfileOptions) (*dockerfile.Dockerfile, error) {
	p, err := parser.Parse(bytes.NewReader(dockerfileBytes))
	if err != nil {
		return nil, fmt.Errorf("parsing dockerfile data: %w", err)
	}

	dockerStages, dockerMetaArgs, err := instructions.Parse(p.AST)
	if err != nil {
		return nil, fmt.Errorf("parsing instructions tree: %w", err)
	}

	dockerTargetIndex, err := GetDockerTargetStageIndex(dockerStages, opts.Target)
	if err != nil {
		return nil, fmt.Errorf("determine target stage: %w", err)
	}

	var stages []*dockerfile.DockerfileStage
	for i, dockerStage := range dockerStages {
		stages = append(stages, DockerfileStageFromBuildkitStage(i, dockerStage))
	}

	// TODO(staged-dockerfile): convert meta-args and initialize into Dockerfile obj
	_ = dockerMetaArgs
	_ = dockerTargetIndex

	dockerfile.SetupDockerfileStagesDependencies(stages)

	d := dockerfile.NewDockerfile(stages, opts)
	for _, stage := range d.Stages {
		stage.Dockerfile = d
	}
	return d, nil
}

func DockerfileStageFromBuildkitStage(index int, stage instructions.Stage) *dockerfile.DockerfileStage {
	var i []dockerfile.DockerfileStageInstructionInterface

	for _, cmd := range stage.Commands {
		switch typedCmd := cmd.(type) {
		case *instructions.AddCommand:
			src, dst := extractSrcAndDst(typedCmd.SourcesAndDest)
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewAdd(src, dst, typedCmd.Chown, typedCmd.Chmod)))
		case *instructions.ArgCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewArg(typedCmd.Args)))
		case *instructions.CmdCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewCmd(typedCmd.CmdLine, typedCmd.PrependShell)))
		case *instructions.CopyCommand:
			src, dst := extractSrcAndDst(typedCmd.SourcesAndDest)
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewCopy(typedCmd.From, src, dst, typedCmd.Chown, typedCmd.Chmod)))
		case *instructions.EntrypointCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewEntrypoint(typedCmd.CmdLine, typedCmd.PrependShell)))
		case *instructions.EnvCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewEnv(extractKeyValuePairsAsMap(typedCmd.Env))))
		case *instructions.ExposeCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewExpose(typedCmd.Ports)))
		case *instructions.HealthCheckCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewHealthcheck(typedCmd.Health)))
		case *instructions.LabelCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewLabel(extractKeyValuePairsAsMap(typedCmd.Labels))))
		case *instructions.MaintainerCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewMaintainer(typedCmd.Maintainer)))
		case *instructions.OnbuildCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewOnBuild(typedCmd.Expression)))
		case *instructions.RunCommand:
			network := dockerfile_instruction.NewNetworkType(instructions.GetNetwork(typedCmd))
			security := dockerfile_instruction.NewSecurityType(instructions.GetSecurity(typedCmd))
			mounts := instructions.GetMounts(typedCmd)
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewRun(typedCmd.CmdLine, typedCmd.PrependShell, mounts, network, security)))
		case *instructions.ShellCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewShell(typedCmd.Shell)))
		case *instructions.StopSignalCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewStopSignal(typedCmd.Signal)))
		case *instructions.UserCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewUser(typedCmd.User)))
		case *instructions.VolumeCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewVolume(typedCmd.Volumes)))
		case *instructions.WorkdirCommand:
			i = append(i, dockerfile.NewDockerfileStageInstruction(dockerfile_instruction.NewWorkdir(typedCmd.Path)))
		}
	}

	return dockerfile.NewDockerfileStage(index, stage.BaseName, stage.Name, i, stage.Platform)
}

func extractSrcAndDst(sourcesAndDest instructions.SourcesAndDest) ([]string, string) {
	if len(sourcesAndDest) < 2 {
		panic(fmt.Sprintf("unexpected buildkit instruction source and destination: %#v", sourcesAndDest))
	}
	dst := sourcesAndDest[len(sourcesAndDest)-1]
	src := sourcesAndDest[0 : len(sourcesAndDest)-1]
	return src, dst
}

func extractKeyValuePairsAsMap(pairs instructions.KeyValuePairs) (res map[string]string) {
	res = make(map[string]string)
	for _, item := range pairs {
		res[item.Key] = item.Value
	}
	return
}

func GetDockerStagesNameToIndexMap(stages []instructions.Stage) map[string]int {
	nameToIndex := make(map[string]int)
	for i, s := range stages {
		name := strings.ToLower(s.Name)
		if name != strconv.Itoa(i) {
			nameToIndex[name] = i
		}
	}
	return nameToIndex
}

func ResolveDockerStagesFromValue(stages []instructions.Stage) {
	nameToIndex := GetDockerStagesNameToIndexMap(stages)

	for _, s := range stages {
		for _, cmd := range s.Commands {
			switch typedCmd := cmd.(type) {
			case *instructions.CopyCommand:
				if typedCmd.From != "" {
					from := strings.ToLower(typedCmd.From)
					if val, ok := nameToIndex[from]; ok {
						typedCmd.From = strconv.Itoa(val)
					}
				}

			case *instructions.RunCommand:
				for _, mount := range instructions.GetMounts(typedCmd) {
					if mount.From != "" {
						from := strings.ToLower(mount.From)
						if val, ok := nameToIndex[from]; ok {
							mount.From = strconv.Itoa(val)
						}
					}
				}
			}
		}
	}
}

func GetDockerTargetStageIndex(dockerStages []instructions.Stage, dockerTargetStage string) (int, error) {
	if dockerTargetStage == "" {
		return len(dockerStages) - 1, nil
	}

	for i, s := range dockerStages {
		if s.Name == dockerTargetStage {
			return i, nil
		}
	}

	return -1, fmt.Errorf("%s is not a valid target build stage", dockerTargetStage)
}