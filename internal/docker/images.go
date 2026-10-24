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
	"strings"
	"sync"

	"github.com/ProfessionalWiki/mwenv/internal/mediawiki"
)

//go:embed build
var buildFiles embed.FS

var phpVersionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){1,2}([.-][A-Za-z0-9]+)*$`)

// imageLabel marks the images mwenv builds, so that PruneImages finds the ones a rebuild replaced.
const imageLabel = "wiki.professional.mwenv=true"

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
		if err := buildPHPImage(php, onBuild); err != nil {
			return err
		}
	}

	return buildCoreImage(version, php, onBuild)
}

// RebuildImages builds the images for a MediaWiki and PHP version again, with the newest commits of the MediaWiki
// branch and the newest PHP base image.
func RebuildImages(version mediawiki.Version, php string, onBuild func(image string)) error {
	if err := buildPHPImage(php, onBuild, "--pull"); err != nil {
		return err
	}

	// Without --no-cache, the build would reuse the clone of the branch from the previous build.
	return buildCoreImage(version, php, onBuild, "--no-cache")
}

func buildPHPImage(php string, onBuild func(image string), flags ...string) error {
	return build(PHPImage(php), "php.Dockerfile", []string{"PHP_VERSION=" + php}, onBuild, flags...)
}

func buildCoreImage(version mediawiki.Version, php string, onBuild func(image string), flags ...string) error {
	buildArgs := []string{"BASE_IMAGE=" + PHPImage(php), "CORE_REF=" + version.CoreRef, "VECTOR_REF=" + version.VectorRef}
	return build(CoreImage(version, php), "core.Dockerfile", buildArgs, onBuild, flags...)
}

// PruneImages removes the images mwenv built that lost their tag to a rebuild and that no container uses. Docker's
// classic image store keeps such images; its containerd image store removes them when the tag moves.
func PruneImages() error {
	_, err := output("image", "prune", "--force", "--filter", "label="+imageLabel)
	// Docker runs one prune at a time, which parallel destroys in several worktrees run into. The prune that runs
	// does this one's work, and whatever it misses the next prune removes.
	if err != nil && strings.Contains(err.Error(), "a prune operation is already running") {
		return nil
	}
	return err
}

func imageExists(tag string) bool {
	_, err := output("image", "inspect", "--format", "{{.Id}}", tag)
	return err == nil
}

func build(image string, dockerfile string, buildArgs []string, onBuild func(image string), flags ...string) error {
	onBuild(image)

	context, err := writeBuildContext()
	if err != nil {
		return err
	}
	defer os.RemoveAll(context)

	args := []string{
		"build", "--progress=plain", "--label", imageLabel,
		"--tag", image, "--file", filepath.Join(context, dockerfile),
	}
	for _, buildArg := range buildArgs {
		args = append(args, "--build-arg", buildArg)
	}
	args = append(args, flags...)

	if _, err := output(append(args, context)...); err != nil {
		return fmt.Errorf("building %s failed: %w", image, err)
	}
	return nil
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
