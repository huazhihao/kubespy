# kubespy: non-invasive debugging tool for kubernetes

[![Build Status](https://travis-ci.org/huazhihao/kubespy.svg?branch=master)](https://travis-ci.org/huazhihao/kubespy)
[![GoDoc](https://godoc.org/github.com/huazhihao/kubespy?status.svg)](https://godoc.org/github.com/huazhihao/kubespy)

`kubespy` is a kubectl plugin which can non-invasively load common system tools into a particular running pod for debugging. So you don't have to modify the spec of the pod or the image of the container just for debugging purpose.


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

  kubespy POD [-c CONTAINER] [--spy SPY_IMAGE]

Examples:

  # spy the first container nginx from mypod
  kubespy mypod

  # sspy container nginx from mypod
  kubespy mypod -c nginx

  # spy container nginx from mypod using busybox
  kubespy mypod -c nginx --spy busybox
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
               sidecar container
                    | (share namespace: pid/net/ipc)
                    v
               target pod (eg. nginx)
```
