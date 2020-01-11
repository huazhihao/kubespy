# kubespy 用bash实现的k8s动态调试工具

## 背景

Kubernetes调试的最大痛点是精简过的容器镜像里没有日常的调试工具。背后的原因是精简容器镜像本身就是容器技术的最佳实践之一。nginx的容器镜像甚至不包含ps和curl这种最基础的工具。这种完全服务于生产环境的策略无异于过早优化，但受制于immutable infrastructure的基本思想和CI/CD实际操作的双重制约，你无法在生产环境发布一个和开发环境不同的容器镜像。这使得这一过早优化的结果更加灾难化。解决这个问题的关键在于，能否在不侵入式的修改容器镜像的情况下，向目标容器里加载需要的调试工具。例如，类似于istio之类的解决方案可以向目标pod插入一个sidecar容器。当然这里的权限要求是高于sidecar容器的，因为pod中的各个容器虽然共享network，但pid和ipc是不共享的。此外,sidecar容器是无法被加入一个已经创建出来的pod，而我们希望工具容器可以在运行时被动态插入，因为问题的产生是随机的，你不能完全预测需要加载哪些工具。

Kubernetes社区很早有相关的[issue](https://github.com/kubernetes/kubernetes/issues/27140)和[proposal](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/node/troubleshoot-running-pods.md)但并没有最后最终被upstream接受。

目前官方给出最接近的方案是[Ephemeral Containers](https://kubernetes.io/docs/concepts/workloads/pods/ephemeral-containers/)和[shareProcessNamespace](https://kubernetes.io/docs/tasks/configure-pod-container/share-process-namespace/)。前者允许你在运行时在一个pod里插入一个短生命周期的容器(无法拥有livenessProbe, readinessProbe)，而后者允许你共享目标pod内容器的network,pid,ipc等cgroups namespace(注意，此namespace非Kubernetes的namespace)甚至修改其中的环境。目前`Ephemeral Containers`还在v1.17 alpha阶段。而且`shareProcessNamespace`这个spec要求在创建pod的时候就必须显式启用，否则运行时无法修改。

[kubespy (https://github.com/huazhihao/kubespy)](https://github.com/huazhihao/kubespy)是一个完全用bash实现的Kubernetes调试工具，它不但完美的解决了上面提到的如何在运行时向目标容器加载工具的问题，而且并不依赖任何最新版本的Kubernetes的特性。这篇文章稍后会介绍如何使用`kubespy`来动态调试，以及`kubespy`是如何通过kubectl,docker以及chrootl来构建这条调试链的，最后会简单分析一下关键的代码实现。

## 安装

你可以直接从代码安装，因为`kubespy`是完全bash实现的，所以可以直接拷贝文件来执行。

```sh
$ curl -so kubectl-spy https://raw.githubusercontent.com/huazhihao/kubespy/master/kubespy
$ sudo install kubectl-spy /usr/local/bin/
```

你也可以从`krew`来安装。`krew`是一个`kubernetes-sigs`孵化中的`kubectl`插件包管理工具，带有准官方性质。

```sh
$ kubectl krew install spy
```

## 使用

安装过后，`kubespy`可以成为一个`kubectl`的子命令被执行。你可以指定目标pod为参数。如果目标pod有多个容器，你可以通过`-c`指定具体的容器。你也可以指定加载的工具容器的镜像，默认是`busybox:latest`

```sh
$ kubectl spy POD [-c CONTAINER] [--spy-image SPY_IMAGE]
```

你可以通过以下这个demo来快速体验如何调试一个镜像为nginx的pod。nginx镜像不包含ps或者任何网络工具。而加载的工具容器的镜像为busybox,不但可以访问原容器的文件系统和进程树，甚至可以杀进程，修改文件。当然发http请求更是没有问题。

[![asciicast](https://asciinema.org/a/290096.svg)](https://asciinema.org/a/290096)

## 工作原理

`kubespy`的工作原理大致可以用以下这个流程图来展示。

```
local machine: kubectl spy [1]
                    |
                    v
master node:   kube-apiserver [2]
                    |
                    v
worker node:   kubelet [3]
                    |
                    v
               spy pod (eg. busybox) [4]
                    | (chroot)
                    v
               docker runtime [5]
                    | (run)
                    v
               spy container [6]
                    | (join docker namespace: pid/net/ipc)
                    v
               application pod (eg. nginx) [7]
```

概要的看，`kubespy`是通过以下这些步骤构建调试连的，上图的步骤数字可以与下文对应

[1] `kubespy`作为`kubectl`的插件被执行，可以向`master node`上的`kube-apiserver`发出api请求,会先取得目标容器的关键信息，如其所在的`worker node`和pid/net/ipc 等cgroups namespace
[2] `kube-apiserver`将具体的命令分发给目标容器所在的`worker node`上的agent`kubelet`执行
[3] `kubelet`创建一个`busybox`作为`spy pod`
[4] `spy pod`mount了`worker node`的根目录，并通过`chroot`取得了worker node的控制权
[5] `spy pod`控制了docker cli创建了工具容器(`spy container `)
[6] 工具容器被加入目标容器的pid/net/ipc等cgroups namespace
[7] 用户通过`kubectl`被attach到工具容器的tty里，可以对目标容器进行调试甚至是修改

## 关键代码

### 如何取得目标容器的pid/net/ipc等cgroups namespace

```sh
  if [[ "${co}" == "" ]]; then
    cid=$(kubectl get pod "${po}" -o='jsonpath={.status.containerStatuses[0].containerID}' | sed 's/docker:\/\///')
  else
    cid=$(kubectl get pod "${po}" -o='jsonpath={.status.containerStatuses[?(@.name=="'"${co}"'")].containerID}' | sed 's/docker:\/\///')
  fi
```

根据`kubectl`的convention,如果用户未指定容器，我们默认以目标pod的第一个容器作为目标容器。`kubelet`在为kubernetes集群创建容器时，会相应的创建以containerID命名的pid/net/ipc等cgroups namespace。cgroups namespace是docker实现user space隔离的基础原理，可以参考https://docs.docker.com/engine/docker-overview/#namespaces 进行了解。

### 如何获得目标容器所在worker node以及其docker cli的控制权

```json
"volumes": [
  {
    "name": "node",
    "hostPath": {
      "path": "/"
    }
  }
]
```
`spy pod`会将`worker node`的根目录作为volume来mount在`/host`

```json
{
  "name": "spy",
  "image": "busybox",
  "command": [ "/bin/chroot", "/host"],
  "args": [
    "docker",
    "run",
    "-it",
    "--network=container:'"${cid}"'",
    "--pid=container:'"${cid}"'",
    "--ipc=container:'"${cid}"'",
    "'"${ep}"'"
  ],
  "stdin": true,
  "stdinOnce": true,
  "tty": true,
  "volumeMounts": [
    {
      "mountPath": "/host",
      "name": "node"
    }
  ]
}
```

然后在通过`busybox`里的chroot，将`worker node`的根目录作为自己的根目录。而docker cli也将被直接暴露出来。

在获取了目标容器的pid/net/ipc等cgroups namespace之后，即可直接创建工具容器共享目标容器的cgroups namespace。此时，目标容器和目标容器在进程树，网络空间，内部进程通讯等，都是没有任何隔离的。

### 如何将用户的terminal带入目标容器中

`kubectl run -it`，`chroot`和`docker run -it`都是可以attach到目标的tty中的，这些命令链接起来，像一系列跳板，把用户的terminal一层层带入下一个，最终带入目标容器中。

## 小结

[kubespy (https://github.com/huazhihao/kubespy)](https://github.com/huazhihao/kubespy) 用bash实现对kubernetes集群中的pod通过动态加载工具容器来调试，弥补了目前kubernetes版本上功能的缺失，并展示了一些对kubernetes本身有深度的技巧。
