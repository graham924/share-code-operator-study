#!/usr/bin/env bash

# Copyright 2017 The Kubernetes Authors.
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

# 设置脚本在执行过程中遇到任何错误时立即退出
set -o errexit
# 设置脚本在使用未定义的变量时立即退出
set -o nounset
# 设置脚本在管道命令中任意一条命令失败时立即退出
set -o pipefail

# 对generate-groups.sh 脚本的调用
../vendor/k8s.io/code-generator/generate-groups.sh \
  "deepcopy,client,informer,lister" \
  code-generator-demo/pkg/generated \
  code-generator-demo/pkg/apis \
  appcontroller:v1alpha1 \
  --go-header-file $(pwd)/boilerplate.go.txt \
  --output-base $(pwd)/../../
