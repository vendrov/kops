#!/usr/bin/env bash
# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

KOPS_ROOT=$(git rev-parse --show-toplevel)
TMP_GOPATH=$(mktemp -d)
cd "${KOPS_ROOT}"

"${KOPS_ROOT}/hack/go_install_from_commit.sh" \
  github.com/bazelbuild/bazel-gazelle/cmd/gazelle \
  578e73e57d6a4054ef933db1553405c9284322c7 \
  "${TMP_GOPATH}"


gazelle_diff=$("${TMP_GOPATH}/bin/gazelle" fix \
  -external=vendored \
  -mode=diff \
  -proto=disable \
  -repo_root="${KOPS_ROOT}")

if [[ -n "${gazelle_diff}" ]]; then
  echo "${gazelle_diff}" >&2
  echo >&2
  echo "FAIL: ./hack/verify-bazel.sh failed, as the bazel files are not up to date" >&2
  echo "FAIL: Please execute the following command: ./hack/update-bazel.sh" >&2
  exit 1
fi

# Make sure there are no BUILD files outside vendor - we should only have
# BUILD.bazel files.
old_build_files=$(find . -name BUILD \( -type f -o -type l \) \
  -not -path './vendor/*' | sort)
if [[ -n "${old_build_files}" ]]; then
  echo "One or more BUILD files found in the tree:" >&2
  echo "${old_build_files}" >&2
  echo >&2
  echo "FAIL: Only bazel files named BUILD.bazel are allowed." >&2
  echo "FAIL: Please move incorrectly named files to BUILD.bazel" >&2
  exit 1
fi
