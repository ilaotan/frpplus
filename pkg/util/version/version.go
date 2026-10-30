// Copyright 2016 fatedier, fatedier@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package version

// version tracks the upstream frp version. Keep it identical to upstream so
// that syncing with upstream never conflicts.
var version = "0.71.1"

// buildVersion is injected at build time by hack/build-release.sh, for example
// -ldflags "-X github.com/fatedier/frp/pkg/util/version.buildVersion=0.71.1.plus.1".
// It carries the fork specific build number; when empty the plain upstream
// version is reported.
var buildVersion = ""

func Full() string {
	if buildVersion != "" {
		return buildVersion
	}
	return version
}
