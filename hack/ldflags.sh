#!/bin/bash

GO_PROJECT="github.com/weaveworks/gitops-toolkit"

# Note: This file is heavily inspired by https://github.com/kubernetes/kubernetes/blob/master/hack/lib/version.sh

get_version_vars() {
  GIT_COMMIT=$(git rev-parse "HEAD^{commit}" 2>/dev/null)
  if git_status=$(git status --porcelain 2>/dev/null) && [[ -z ${git_status} ]]; then
    GIT_TREE_STATE="clean"
  else
    GIT_TREE_STATE="dirty"
  fi
  # Use git describe to find the version based on tags.
  GIT_VERSION=$(git describe --tags --abbrev=14 "${GIT_COMMIT}^{commit}" 2>/dev/null)

  # This translates the "git describe" to an actual semver.org
  # compatible semantic version that looks something like this:
  #   v1.1.0-alpha.0.6+84c76d1142ea4d
  DASHES_IN_VERSION=$(echo "${GIT_VERSION}" | sed "s/[^-]//g")
  if [[ "${DASHES_IN_VERSION}" == "---" ]] ; then
    # We have distance to subversion (v1.1.0-subversion-1-gCommitHash)
    GIT_VERSION=$(echo "${GIT_VERSION}" | sed "s/-\([0-9]\{1,\}\)-g\([0-9a-f]\{14\}\)$/.\1\+\2/")
  elif [[ "${DASHES_IN_VERSION}" == "--" ]] ; then
    # We have distance to base tag (v1.1.0-1-gCommitHash)
    GIT_VERSION=$(echo "${GIT_VERSION}" | sed "s/-g\([0-9a-f]\{14\}\)$/+\1/")
  fi
  if [[ "${GIT_TREE_STATE}" == "dirty" ]]; then
    # git describe --dirty only considers changes to existing files, but
    # that is problematic since new untracked .go files affect the build,
    # so use our idea of "dirty" from git status instead.
    GIT_VERSION+="-dirty"
  fi

  # Try to match the "git describe" output to a regex to try to extract
  # the "major" and "minor" versions and whether this is the exact tagged
  # version or whether the tree is between two tagged versions.
  if [[ "${GIT_VERSION}" =~ ^v([0-9]+)\.([0-9]+)(\.[0-9]+)?([-].*)?([+].*)?$ ]]; then
    GIT_MAJOR=${BASH_REMATCH[1]}
    GIT_MINOR=${BASH_REMATCH[2]}
    if [[ -n "${BASH_REMATCH[4]}" ]]; then
      GIT_MINOR+="+"
    fi
  fi
}

ldflag() {
  local key=${1}
  local val=${2}
  echo "-X '${GO_PROJECT}/pkg/version.${key}=${val}'"
}

# Prints the value that needs to be passed to the -ldflags parameter of go build
# in order to set the version based on the git tree status.
ldflags() {
  get_version_vars

  local buildDate=
  [[ -z ${SOURCE_DATE_EPOCH-} ]] || buildDate="--date=@${SOURCE_DATE_EPOCH}"
  local -a ldflags=($(ldflag "buildDate" "$(date ${buildDate} -u +'%Y-%m-%dT%H:%M:%SZ')"))
  if [[ -n ${GIT_COMMIT-} ]]; then
    ldflags+=($(ldflag "gitCommit" "${GIT_COMMIT}"))
    ldflags+=($(ldflag "gitTreeState" "${GIT_TREE_STATE}"))
  fi

  if [[ -n ${GIT_VERSION-} ]]; then
    ldflags+=($(ldflag "gitVersion" "${GIT_VERSION}"))
  fi

  if [[ -n ${GIT_MAJOR-} && -n ${GIT_MINOR-} ]]; then
    ldflags+=(
      $(ldflag "gitMajor" "${GIT_MAJOR}")
      $(ldflag "gitMinor" "${GIT_MINOR}")
    )
  fi

  # Output only the version with this flag
  if [[ $1 == "--version-only" ]]; then
    echo "${GIT_VERSION}"
    exit 0
  fi

  # The -ldflags parameter takes a single string, so join the output.
  echo "${ldflags[*]-}"
}

ldflags $@