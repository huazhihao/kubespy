# kubespy: non-invasive debugging tool for kubernetes

[![Build Status](https://travis-ci.org/huazhihao/kubespy.svg?branch=master)](https://travis-ci.org/huazhihao/kubespy)
![Proudly written in Bash](https://img.shields.io/badge/written%20in-bash-ff69b4.svg)

`kubespy` is a kubectl plugin which creates and runs an ephemeral toolbox container mounting on the pid/net/ipc namespace of a particular pod for debugging during runtime. So you don't have to bundle the tools with your image just for debugging purpose.


## Examples

[![asciicast](https://asciinema.org/a/290096.svg)](https://asciinema.org/a/290096)

## Installation

```sh
curl -so kubectl-spy https://raw.githubusercontent.com/huazhihao/kubespy/master/kubespy
sudo install kubectl-spy /usr/local/bin/
```

## Usage

```
Load common system tools into a particular running pod for debugging

Usage:

  kubectl spy POD [-c CONTAINER] [--ephemeral TOOLBOX_IMAGE]

Examples:

  # spy the first container nginx from mypod
  kubectl spy mypod

  # spy container nginx from mypod
  kubectl spy mypod -c nginx

  # spy container nginx from mypod using busybox
  kubectl spy mypod -c nginx --ephemeral busybox
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
               ephemeral container
                    | (share namespace: pid/net/ipc)
                    v
               target pod (eg. nginx)
```
