# kubespy

[![Build Status](https://travis-ci.org/huazhihao/kubespy.svg?branch=master)](https://travis-ci.org/huazhihao/kubespy)
![Proudly written in Bash](https://img.shields.io/badge/written%20in-bash-ff69b4.svg)

`kubespy` is a kubectl plugin implemented in bash to debug a running pod. It starts a temporary `spy container` which joins the namespaces of the target container (eg. pid/net/ipc). You can specify the image of `spy container` which should include all the required debugging tools. Thus, the debugging tools need not unnecessarily be bundled with the main application container image.

`kubespy` is similar to [kubectl-debug](https://github.com/verb/kubectl-debug). In contrast to the latter, kubespy works without the EphemeralContainers feature which is an experimental alpha feature and needs to be activated per pod.

Meanwhile `kubespy` has its prerequisites - the cluster must use docker as container runtime and you need to be able to run privileged pods.

## Installation

```sh
$ curl -so kubectl-spy https://raw.githubusercontent.com/huazhihao/kubespy/master/kubespy
$ sudo install kubectl-spy /usr/local/bin/
```

## Usage

```sh
$ kubectl spy POD [-c CONTAINER] [--spy-image SPY_IMAGE]
```

## Examples:

[![asciicast](https://asciinema.org/a/290096.svg)](https://asciinema.org/a/290096)

```sh
# debug the first container nginx from mypod
$ kubectl spy mypod

# debug container nginx from mypod
$ kubectl spy mypod -c nginx

# debug container nginx from mypod using busybox
$ kubectl spy mypod -c nginx --spy-image busybox
```

## Architecture

```
local machine: kubectl spy
                    |
                    v
master node:   kube-apiserver
                    |
                    v
worker node:   kubelet
                    |
                    v
               spy pod (eg. busybox)
                    | (chroot)
                    v
               docker runtime
                    | (run)
                    v
               spy container
                    | (join docker namespace: pid/net/ipc)
                    v
               application pod (eg. nginx)
```
