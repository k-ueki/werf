package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/storage/pkg/reexec"
	"github.com/containers/storage/pkg/unshare"
	"github.com/sirupsen/logrus"
	"github.com/werf/werf/pkg/buildah"
)

func init() {
	logrus.SetLevel(logrus.TraceLevel)

	unshare.MaybeReexecUsingUserNamespace(false)
}

func main() {
	if reexec.Init() {
		return
	}

	b, err := buildah.NewBuildah(buildah.ModeNativeRootless)
	if err != nil {
		panic(err.Error())
	}

	if err := do(b); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	}

	if imageId, err := do2(b); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	} else {
		fmt.Fprintf(os.Stdout, "INFO: imageId is %s\n", imageId)
	}
}

func do(b buildah.Buildah) error {
	return b.RunCommand(context.Background(), "build-container", []string{"ls"}, buildah.RunCommandOpts{})
}

func do2(b buildah.Buildah) (string, error) {
	tarFileReader, err := os.Open(filepath.Join(os.Getenv("HOME"), "/tmp/werf-buildah/context.tar"))
	if err != nil {
		return "", err
	}
	defer tarFileReader.Close()

	dockerfileContent, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), "/tmp/werf-buildah/Dockerfile"))
	if err != nil {
		return "", err
	}

	return b.BuildFromDockerfile(
		context.Background(),
		buildah.Dockerfile{
			Content:        dockerfileContent,
			ContextRelPath: "Dockerfile",
		},
		buildah.BuildFromDockerfileOpts{
			ContextTar: tarFileReader,
		})
}
