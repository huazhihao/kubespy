# -*- mode: ruby -*-
# vi: set ft=ruby :

Vagrant.configure("2") do |config|
  config.vm.box = "hashicorp/bionic64"
  config.vm.network "public_network"

  config.vm.provider "virtualbox" do |vb|
    vb.cpus = 2
    vb.memory = "2048"
  end
  config.vm.provision "shell", inline: <<-SHELL
    mkdir -p /usr/local/bin/

    curl -LO https://storage.googleapis.com/kubernetes-release/release/`curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt`/bin/linux/amd64/kubectl
    chmod +x kubectl
    install kubectl /usr/local/bin/
    rm kubectl -f
    kubectl version

    curl -sLo minikube https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64
    chmod +x minikube
    install minikube /usr/local/bin/
    rm minikube -f
    minikube version

    curl -fsSL https://get.docker.com | sh
    usermod -aG docker $USER

    minikube start  --vm-driver=none
    mv /home/vagrant/.kube /home/vagrant/.minikube $HOME
    chown -R $USER $HOME/.kube $HOME/.minikube
  SHELL
end
