# kubespy

[![Build Status](https://travis-ci.org/huazhihao/kubespy.svg?branch=master)](https://travis-ci.org/huazhihao/kubespy)
![Proudly written in Bash](https://img.shields.io/badge/written%20in-bash-ff69b4.svg)
[![LICENSE](https://img.shields.io/github/license/huazhihao/kubespy.svg)](https://github.com/huazhihao/kubespy/blob/master/LICENSE)
[![Releases](https://img.shields.io/github/v/release/huazhihao/kubespy.svg)](https://github.com/huazhihao/kubespy/releases)

`kubespy` is a kubectl plugin to debug a running pod without any prerequisites. It creates a short-lived `spy container`, which contains all the required debugging tools, to "spy" the target container by joining its namespaces. So the target container image can keep clean without sacrificing the convenience for debugging on demond.

`kubespy` is similar to [kubectl-debug](https://github.com/verb/kubectl-debug). In contrast to the latter, kubespy works without the EphemeralContainers feature which is an experimental alpha feature and needs to be activated per pod.

Meanwhile `kubespy` has its own prerequisites - the cluster must use docker as container runtime and you need to be able to run privileged pods.

## Installation

You can install either from source or with `krew`

### Install from source

```sh
$ curl -so kubectl-spy https://raw.githubusercontent.com/huazhihao/kubespy/master/kubespy
$ sudo install kubectl-spy /usr/local/bin/
```

### Install with `krew`

```sh
$ kubectl krew install spy
```

## Usage

```sh
$ kubectl spy POD [-c CONTAINER] [-n NAMESPACE] [--spy-image SPY_IMAGE]
```

## Examples:

[![asciicast](https://asciinema.org/a/290096.svg)](https://asciinema.org/a/290096)

```sh
# debug the primary (first) container in pod mypod
$ kubectl spy mypod

# specify pod namespace
$ kubectl spy mypod -n default

# specify debugee container
$ kubectl spy mypod -c mycontainer

# specify spy-image
$ kubectl spy mypod --spy-image busybox

# specify entrypoint for interaction
$ kubectl spy mypod --entrypoint /bin/sh

# specify additional port to be exposed
$ kubectl spy mypod -p 2345
```

## Workflow

```
local machine: kubectl spy
                    |
                    v
master node:   kube-apiserver
                    |
                    v
worker node:   kubelet
                    | create
                    v
               spy container
                    | join namespace: pid/net/ipc/mount/uts
                    v
               target container
```
