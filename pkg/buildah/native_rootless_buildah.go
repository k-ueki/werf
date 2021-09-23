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
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/unshare"
	"github.com/werf/logboek"
	"gopkg.in/errgo.v2/errors"
)

type NativeRootlessBuildah struct {
	store storage.Store
}

func NewNativeRootlessBuildah() (*NativeRootlessBuildah, error) {
	buildah := &NativeRootlessBuildah{}

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

func (b *NativeRootlessBuildah) BuildFromDockerfile(ctx context.Context, dockerfile Dockerfile, opts BuildFromDockerfileOpts) (string, error) {
	// REVIEW(ilya-lesikov): other path for the temp dir?
	contextTmpDir, err := ioutil.TempDir("", "werf-buildah")
	if err != nil {
		return "", fmt.Errorf("unable to prepare contextTmpDir: %s", err)
	}
	defer func() {
		if err = os.RemoveAll(contextTmpDir); err != nil {
			logboek.Warn().LogF("unable to remove contextTmpDir %q: %s\n", contextTmpDir, err)
		}
	}()

	if opts.ContextTar != nil {
		if err := ExtractTar(opts.ContextTar, contextTmpDir); err != nil {
			return "", fmt.Errorf("unable to extract context tar to temp context dir: %s", err)
		}
	}

	if err := os.WriteFile(filepath.Join(contextTmpDir, dockerfile.ContextRelPath), dockerfile.Content, 0644); err != nil {
		return "", fmt.Errorf("unable to write Dockerfile %q to contextTmpDir %q: %s\n", dockerfile.ContextRelPath, contextTmpDir, err)
	}

	// FIXME(ilya-lesikov): capture output?
	buildOpts := define.BuildOptions{
		ContextDirectory: contextTmpDir,
		Isolation:        define.IsolationOCIRootless,
		CommonBuildOpts: &define.CommonBuildOptions{
			ShmSize: DefaultShmSize,
		},
	}

	imageId, _, err := imagebuildah.BuildDockerfiles(ctx, b.store, buildOpts, dockerfile.ContextRelPath)
	if err != nil {
		return "", fmt.Errorf("unable to build Dockerfile %q: %s", dockerfile.ContextRelPath, err)
	}

	return imageId, nil
}

// FIXME(ilya-lesikov):
func (b *NativeRootlessBuildah) RunCommand(ctx context.Context, container string, command []string, opts RunCommandOpts) error {
	runOpts := buildah.RunOptions{
		Args: opts.BuildArgs,
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
func ExtractTar(tarFileReader io.Reader, dstDir string) error {
	tarReader := tar.NewReader(tarFileReader)
	for {
		tarEntryHeader, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("unable to Next() while extracting tar: %s", err)
		}

		tarEntryPath := filepath.Join(dstDir, tarEntryHeader.Name)
		tarEntryFileInfo := tarEntryHeader.FileInfo()

		switch tarEntryHeader.Typeflag {
		case tar.TypeDir:
			if err = os.MkdirAll(tarEntryPath, tarEntryFileInfo.Mode()); err != nil {
				return fmt.Errorf("unable to create new dir %q while extracting tar: %s", tarEntryPath, err)
			}
		default:
			file, err := os.OpenFile(tarEntryPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, tarEntryFileInfo.Mode())
			if err != nil {
				return fmt.Errorf("unable to create new file %q while extracting tar: %s", tarEntryPath, err)
			}
			defer file.Close()

			_, err = io.Copy(file, tarReader)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
