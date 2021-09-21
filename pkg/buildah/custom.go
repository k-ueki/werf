package buildah

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/containers/buildah"
	"github.com/containers/buildah/define"
	"github.com/containers/buildah/imagebuildah"
	is "github.com/containers/image/v5/storage"
	"github.com/containers/storage/pkg/unshare"
	"github.com/werf/logboek"

	"github.com/containers/storage"
	"github.com/pkg/errors"
)

/*
// FIXME(ilya-lesikov):
InitProcess() {
	if reexec.Init() {
		return
	}

	unshare.MaybeReexecUsingUserNamespace(false)
}

Init(Options) -> BuildahObject {
	parse options & init store
}
BuildahObject.Bud(ctx, ...) // обязательно принимать dockerfile через []byte, т.к. гитерминизм
BuildahObject.Run(ctx)
...
*/

const DefaultShmSize = "65536k"

type Buildah struct {
	store storage.Store
}

type BuildFromDockerfileOpts struct {
	ContextTarPath    string
	DockerfileRelPath string
}

type RunCommandOpts struct {
	buildArgs []string
}

// FIXME(ilya-lesikov):
// type BuildahOpts struct {}

func NewBuildah() (*Buildah, error) {
	buildah := &Buildah{}

	storeOpts, err := storage.DefaultStoreOptions(unshare.IsRootless(), unshare.GetRootlessUID())
	if err != nil {
		return nil, fmt.Errorf("unable to set default storage opts: %s", err)
	}

	buildah.store, err = storage.GetStore(storeOpts)
	if err != nil {
		return nil, fmt.Errorf("unable to get storage: %s", err)
	}
	is.Transport.SetStore(buildah.store)

	return buildah, nil
}

// FIXME(ilya-lesikov): capture output?
func (b *Buildah) BuildFromDockerfile(ctx context.Context, opts BuildFromDockerfileOpts) (string, error) {
	// REVIEW(ilya-lesikov): other path for the temp dir?
	contextTmpDir, err := ioutil.TempDir("", "werf-buildah")
	if err != nil {
		return "", fmt.Errorf("unable to prepare temp context dir: %s", err)
	}
	defer func() {
		if err = os.RemoveAll(contextTmpDir); err != nil {
			logboek.Warn().LogF("unable to remove temp context dir: %s\n", err)
		}
	}()

	if err := ExtractTar(opts.ContextTarPath, contextTmpDir); err != nil {
		return "", fmt.Errorf("unable to extract context tar to temp context dir: %s", err)
	}

	io.Copy()

	buildOpts := define.BuildOptions{
		ContextDirectory: contextTmpDir,
		Isolation:        define.IsolationOCIRootless,
		CommonBuildOpts: &define.CommonBuildOptions{
			ShmSize: DefaultShmSize,
		},
	}

	imageId, _, err := imagebuildah.BuildDockerfiles(ctx, b.store, buildOpts, opts.DockerfileRelPath)
	if err != nil {
		return "", fmt.Errorf("unable to build dockerfile: %s", err)
	}

	return imageId, nil
}

func (b *Buildah) RunCommand(ctx context.Context, container string, command []string, opts RunCommandOpts) error {
	runOpts := buildah.RunOptions{
		Args: opts.buildArgs,
	}

	builder, err := buildah.OpenBuilder(b.store, container)
	switch {
	case os.IsNotExist(errors.Cause(err)):
		builder, err = buildah.ImportBuilder(ctx, b.store, buildah.ImportOptions{
			Container: container,
		})
		if err != nil {
			return fmt.Errorf("unable to import builder for container %q: %s", container, err)
		}
	case err != nil:
		return fmt.Errorf("unable to open builder for container %q: %s", container, err)
	}

	return builder.Run(command, runOpts)
}

// FIXME(ilya-lesikov): clean up, move out
func ExtractTar(tarPath, dstDir string) error {
	reader, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		path := filepath.Join(dstDir, header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}

	return nil
}
