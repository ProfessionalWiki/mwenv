package mediawiki

import (
	"fmt"
	"regexp"
	"strconv"
)

const minimumMinorVersion = 43

var (
	releaseBranchPattern  = regexp.MustCompile(`^(?:1\.|REL1_)(\d+)$`)
	releaseVersionPattern = regexp.MustCompile(`^1\.(\d+)\.\d+(-rc\.\d+)?$`)
	nonTagCharacters      = regexp.MustCompile(`[^A-Za-z0-9_.-]`)
)

type Version struct {
	Requested string
	CoreRef   string
	VectorRef string
}

func ParseVersion(requested string) (Version, error) {
	if match := releaseBranchPattern.FindStringSubmatch(requested); match != nil {
		return releaseBranchVersion(requested, match[1])
	}

	if match := releaseVersionPattern.FindStringSubmatch(requested); match != nil {
		v, err := releaseBranchVersion(requested, match[1])
		if err != nil {
			return Version{}, err
		}

		v.CoreRef = requested
		return v, nil
	}

	if requested == "" {
		return Version{}, fmt.Errorf("the MediaWiki version must not be empty")
	}

	return Version{Requested: requested, CoreRef: requested, VectorRef: "master"}, nil
}

func releaseBranchVersion(requested string, minor string) (Version, error) {
	minorNumber, err := strconv.Atoi(minor)
	if err != nil || minorNumber < minimumMinorVersion {
		return Version{}, fmt.Errorf("MediaWiki %s is not supported; the oldest supported version is 1.%d", requested, minimumMinorVersion)
	}

	branch := "REL1_" + minor
	return Version{Requested: requested, CoreRef: branch, VectorRef: branch}, nil
}

// TagPart is the requested version in the characters a Docker image tag allows.
func (v Version) TagPart() string {
	return nonTagCharacters.ReplaceAllString(v.Requested, "-")
}
