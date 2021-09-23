package buildah

import (
	"context"
	"fmt"
	"io"

	"github.com/containers/storage/pkg/unshare"
	"github.com/docker/docker/pkg/reexec"
)

const DefaultShmSize = "65536k"

type CommonOpts struct {
	LogWriter io.Writer
}

type BuildFromDockerfileOpts struct {
	CommonOpts
	ContextTar io.Reader
}

type RunCommandOpts struct {
	CommonOpts
	BuildArgs []string
}

type Dockerfile struct {
	Content        []byte
	ContextRelPath string
}

type Buildah interface {
	BuildFromDockerfile(ctx context.Context, dockerfile Dockerfile, opts BuildFromDockerfileOpts) (string, error)
	RunCommand(ctx context.Context, container string, command []string, opts RunCommandOpts) error
}

type Mode int

const (
	ModeAuto Mode = iota
	ModeNativeRootless
	ModeDockerWithFuse
)

func NewBuildah(mode Mode) (Buildah, error) {
	switch mode {
	case ModeAuto:
		// TODO: auto select based on OS
		return nil, nil
	case ModeNativeRootless:
		// TODO: validate selected mode with OS
		buildah, err := NewNativeRootlessBuildah()
		if err != nil {
			return nil, fmt.Errorf("unable to create new Buildah instance with mode %d: %s", mode, err)
		}
		return buildah, nil
	case ModeDockerWithFuse:
		// TODO: validate selected mode with OS
		return NewDockerWithFuseBuildah()
	default:
		panic(fmt.Sprintf("unexpected Mode: %d", mode))
	}
}

// FIXME(ilya-lesikov):
func ReexecProcess(mode Mode) bool {
	switch mode {
	case ModeNativeRootless:
		unshare.MaybeReexecUsingUserNamespace(false)
		if reexec.Init() {
			return true
		}
	case ModeDockerWithFuse:
		panic("not implemented")
	default:
		panic(fmt.Sprintf("unexpected Mode: %d", mode))
	}

	return false
}
