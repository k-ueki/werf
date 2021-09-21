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

var buildahInstance *buildah.Buildah

func init() {
	logrus.SetLevel(logrus.TraceLevel)

	unshare.MaybeReexecUsingUserNamespace(false)

	b, err := buildah.NewBuildah()
	if err != nil {
		panic(err.Error())
	}

	buildahInstance = b
}

func main() {
	if reexec.Init() {
		return
	}

	// if err := do(); err != nil {
	// 	fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	// }

	if imageId, err := do2(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
	} else {
		fmt.Fprintf(os.Stdout, "INFO: imageId is %s\n", imageId)
	}
}

func do() error {
	return buildahInstance.RunCommand(context.Background(), "build-container", []string{"ls"}, buildah.RunCommandOpts{})
}
func do2() (string, error) {
	return buildahInstance.BuildFromDockerfile(context.Background(), buildah.BuildFromDockerfileOpts{
		ContextTarPath:    filepath.Join(os.Getenv("HOME"), "/tmp/werf-buildah/context.tar"),
		DockerfileRelPath: filepath.Join(os.Getenv("HOME"), "/tmp/werf-buildah/Dockerfile"),
	})
}
