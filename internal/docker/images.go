package docker

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/ProfessionalWiki/mwenv/internal/mediawiki"
)

//go:embed build
var buildFiles embed.FS

var phpVersionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){1,2}([.-][A-Za-z0-9]+)*$`)

func ValidatePHPVersion(version string) error {
	if !phpVersionPattern.MatchString(version) {
		return fmt.Errorf("%q is not a PHP version such as 8.3", version)
	}
	return nil
}

func PHPImage(php string) string {
	return "mwenv-php:" + php + "-" + phpHash()
}

func CoreImage(version mediawiki.Version, php string) string {
	return "mwenv-core:" + version.TagPart() + "-php" + php + "-" + coreHash()
}

// Each tag ends with a hash of the build files of its image, so that a release that changes them builds new images.
// The core image is built on the PHP image, so its hash covers the PHP image's files too.
var (
	phpHash  = sync.OnceValue(func() string { return hashBuildFiles("php.Dockerfile") })
	coreHash = sync.OnceValue(func() string {
		return hashBuildFiles("php.Dockerfile", "core.Dockerfile", "apache.conf", "php.ini")
	})
)

func hashBuildFiles(names ...string) string {
	hash := sha256.New()
	for _, name := range names {
		content, _ := buildFiles.ReadFile("build/" + name)
		hash.Write([]byte(name))
		hash.Write(content)
	}
	return hex.EncodeToString(hash.Sum(nil))[:8]
}

// EnsureImages builds the images for a MediaWiki and PHP version if they are missing, calling onBuild before each
// build.
func EnsureImages(version mediawiki.Version, php string, onBuild func(image string)) error {
	core := CoreImage(version, php)
	if imageExists(core) {
		return nil
	}

	base := PHPImage(php)
	if !imageExists(base) {
		onBuild(base)
		if err := build(base, "php.Dockerfile", "PHP_VERSION="+php); err != nil {
			return fmt.Errorf("building %s failed: %w", base, err)
		}
	}

	onBuild(core)
	err := build(core, "core.Dockerfile", "BASE_IMAGE="+base, "CORE_REF="+version.CoreRef, "VECTOR_REF="+version.VectorRef)
	if err != nil {
		return fmt.Errorf("building %s failed: %w", core, err)
	}
	return nil
}

func imageExists(tag string) bool {
	_, err := output("image", "inspect", "--format", "{{.Id}}", tag)
	return err == nil
}

func build(tag string, dockerfile string, buildArgs ...string) error {
	context, err := writeBuildContext()
	if err != nil {
		return err
	}
	defer os.RemoveAll(context)

	args := []string{"build", "--progress=plain", "--tag", tag, "--file", filepath.Join(context, dockerfile)}
	for _, buildArg := range buildArgs {
		args = append(args, "--build-arg", buildArg)
	}

	_, err = output(append(args, context)...)
	return err
}

func writeBuildContext() (string, error) {
	context, err := os.MkdirTemp("", "mwenv-build-")
	if err != nil {
		return "", err
	}

	subtree, err := fs.Sub(buildFiles, "build")
	if err != nil {
		return "", err
	}

	if err := os.CopyFS(context, subtree); err != nil {
		_ = os.RemoveAll(context)
		return "", err
	}

	return context, nil
}
