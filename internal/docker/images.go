package docker

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
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

// imageLabel marks the images mwenv builds, so that PruneImages finds the ones it no longer uses.
const imageLabel = "wiki.professional.mwenv=true"

// buildHashPattern matches the hash of the build files at the end of an image tag.
var buildHashPattern = regexp.MustCompile(`-([0-9a-f]{8})$`)

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

// PruneImages removes the images mwenv built that no container uses and that this version of mwenv would not use:
// those that other versions of mwenv built from other build files, and those that lost their tag to a rebuild.
// Docker's classic image store keeps the latter; its containerd image store removes them when the tag moves.
func PruneImages() error {
	otherBuildsErr := removeOtherBuilds()

	_, err := output("image", "prune", "--force", "--filter", "label="+imageLabel)
	// Docker runs one prune at a time, which parallel destroys in several worktrees run into. The prune that runs
	// does this one's work, and whatever it misses the next prune removes.
	if err != nil && strings.Contains(err.Error(), "a prune operation is already running") {
		err = nil
	}
	return errors.Join(otherBuildsErr, err)
}

// removeOtherBuilds removes the tags of the images built from other build files. Docker and Podman refuse to remove
// an image that a container uses, so an environment that another version of mwenv created keeps its image until it
// is destroyed. Podman only untags a PHP image that a core image is built on, which leaves it to the prune.
func removeOtherBuilds() error {
	listing, err := output("image", "ls", "--filter", "label="+imageLabel, "--format", "{{.Repository}}:{{.Tag}}")
	if err != nil {
		return err
	}

	for _, image := range otherBuilds(listing, currentHashes()) {
		// A failure, such as the refusal for an image in use, leaves the image to a later prune.
		_, _ = output("image", "rm", image)
	}
	return nil
}

// currentHashes maps the repository of each image to the hash that this version of mwenv puts in its tags.
func currentHashes() map[string]string {
	return map[string]string{"mwenv-php": phpHash(), "mwenv-core": coreHash()}
}

// otherBuilds returns the images in a listing of repository:tag lines whose tag ends with another hash than the
// current one of their repository. Podman lists local images under localhost/.
func otherBuilds(listing string, hashes map[string]string) []string {
	var images []string
	for _, line := range strings.Split(listing, "\n") {
		image := strings.TrimSpace(line)
		separator := strings.LastIndex(image, ":")
		if separator < 0 {
			continue
		}
		current, ours := hashes[strings.TrimPrefix(image[:separator], "localhost/")]
		match := buildHashPattern.FindStringSubmatch(image[separator+1:])
		if ours && match != nil && match[1] != current {
			images = append(images, image)
		}
	}
	return images
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
