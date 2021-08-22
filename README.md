# kubespy

[![Build Status](https://github.com/huazhihao/kubespy/actions/workflows/test.yml/badge.svg)](https://github.com/huazhihao/kubespy/actions/workflows/test.yml)
[![LICENSE](https://img.shields.io/github/license/huazhihao/kubespy.svg)](https://github.com/huazhihao/kubespy/blob/master/LICENSE)
[![Releases](https://img.shields.io/github/v/release/huazhihao/kubespy.svg)](https://github.com/huazhihao/kubespy/releases)

`kubespy` is a kubectl plugin to debug a running pod. It creates a short-lived `spy container`, using specified image containing all the required debugging tools, to "spy" the target container by joining its OS namespaces. So the original target container image can keep clean without sacrificing the convenience for debugging on demand.

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
$ kubectl spy [-c CONTAINER] [-n NAMESPACE] [--spy-image IMAGE] POD
```

## Examples

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
                    | join namespace: pid/net/ipc/mount
                    v
               target container
```

## Advanced Usage

### Remote debugging

By using kubespy and kubeproxy together, will be able to expose port for remote debugging on a running pod.

Below is an example illustrating how to remote debug a Go http server.

```sh
$ kubectl run --image kubespy-port-demo --port=8000 --restart=Never mypod --dry-run=client -o yaml > kubespy-port-demo.yaml
```

Add `imagePullPolicy: Never` if you want kubelet to use local image.

Create pod and service.

```sh
$ kubectl apply -f kubespy-port-demo.yaml
$ kubectl expose pod mypod --port=8000 --name=mypod
$ kubectl get svc
NAME    TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
mypod   ClusterIP   10.111.9.130   <none>        8000/TCP   9s

$ curl http://10.111.9.130:8000
{"message":"hello"}
```

Run `kubespy` and install dlv. dlv's remote debugging protocol is json/rpc via tcp/streaming.

```sh
$ ./kubespy --spy-image golang mypod
loading spy pod spy-7bdaf6b74933 ...
If you don't see a command prompt, try pressing enter.

# go install github.com/go-delve/delve/cmd/dlv@latest
```

In this case, ptrace is required on node.

```sh
echo 0 > /proc/sys/kernel/yama/ptrace_scope
```

Run dlv to attach Go process.

Note: dlv will ignore most OS signals except SIGKILL. In order to close dlv and spy container after, it is better run dlv at background.

```
# ps aux
USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
root         1  0.0  0.3 708144  7012 ?        Ssl  03:29   0:00 /go/bin/kubespy-port-demo
root        11  0.4  0.0   2416   576 pts/0    Ss   03:34   0:00 /bin/sh
root        18  0.0  0.1   6696  2892 pts/0    R+   03:34   0:00 ps aux

# dlv --listen=:2345 --headless=true --log=true --log-output=debugger,debuglineerr,gdbwire,lldbout,rpc --api-version=2 --accept-multiclient attach 1 &>> dlv.log &
```

Run kubeproxy to forward debug port

```
$ kubectl port-forward --address 0.0.0.0 mypod 2345
Forwarding from 0.0.0.0:2345 -> 2345
```

Suppose you are using vscode. Create `.vscode/launch.json` with below content.

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "go",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "port": 2345,
      "host": "your_kubeproxy_IP",
      "trace": "verbose"
    }
  ]
}
```

Start debugging in vscode

![dlv-screenshot](etc/dlv-screenshot.png?raw=true)
