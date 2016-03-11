Ok, time to update these steps.  I have a long list of notes I've taken on setting up and running my desktop.
I'll list them here and try to order them.

# Table Of Contents

| Section                                                          |Section 2                                                            |
|------------------------------------------------------------------|---------------------------------------------------------------------|
| [Ubuntu Post Setup](#ubuntu-post-setup)                          | [HP Elitebook 8500 special setup](#hp-elitebook-8500-special-setup) |
| [Install software update tools](#install-software-update-tools)  | [Package installs](#package-installs)                               |
| [Install build tools](#install-build-tools)                      | [Install go lang](#install-go-lang)                                 |
| [Bootstrap puppet](#bootstrap-puppet)                            | [MyHome directory setup](#myhome-directory-setup)                   |
| [git shortcuts](#git-shortcuts)                                  | [setup zsh](#setup-zsh)                                             |
| [give your command prompt some bling](#give-your-command-prompt-some-bling)| [key bindings setup](#key-bindings-setup)                 |
| [Install google drive sync](#install-google-drive-sync)          | [install nodejs](#install-nodejs)                                |
| [install atom](#install-atom)                                    | [install wine, with wow](#install-wine-with-wow)                 |
| [install myrooms](#install-myrooms)                              | [pidgin installation](#pidgin-installation)                      |
| [install dig](#install-dig)                                      | [Installing extra certificates](#installing-extra-certificates)  |
| [install hipchat](#install-hipchat)                              | [install java](#install-java)                                    |
| [setup latest version of seahorse for gnome keyring](#setup-latest-version-of-seahorse-for-gnome-keyring)| [setup shellcheck](#setup-shellcheck)                            |
| [Install Google Chrome](#install-google-chrome)                  | [Configure Google Chrome App Launcher](#configure-google-chrome-app-launcher)                   |
| [chrombook kernel headers installation](#chrombook-kernel-headers-installation)| [Install virtualbox 5 (experimental)](#install-virtualbox-5-experimental)     |
| [Win 8.1/2012 images don't boot for installation](#win-812012-images-dont-boot-for-installation)| [Install vagrant](#install-vagrant)                              |
| [Install forj-docker](#install-forj-docker)                      | [private vpn setup](#private-vpn-setup)                          |
| [chage reboot policy in](#change-rboot-policy-in)                | [Setup sshd server](#setup-sshd-server)                          |
| [installing python](#installing-python)                          | [installing pip](#installing-pip)                                |
| [installing openstack clients](#installing-openstack-clients)    | [install ruby](#install-ruby)|

Ubuntu Post Setup
-----------------
These instructions are mainly for my setup but might apply to others who are doing the same.  Feel free to borrow as needed.

I've mainly been using these steps on Ubuntu 14.04 standalone and versions I install from cruton on ChromeOS

#### HP Elitebook 8500 special setup ####
Setup nvidia drivers and bios options.  Only need to perform for Elitebook 8500 laptops.  Otherwise the suspend feature will fail to work correctly with ubuntu.

1. edit /etc/defaults/grub, add line after quiet splash, nouveau.modeset=0
2. update grub: ```sudo update-grub```

Configure the video drivers

1. open system settings -> drivers -> software & update -> Additional Drivers
2. use driver: NVIDIDA binary driver - version 331.113 from nvidia-331 (proprietary, tested)
3. ```sudo reboot```

Configure the system bios by disable fastboot, or use bios defaults

#### Install software update tools ####
```script
sudo apt-get update; \
sudo apt-get -y autoremove ; \
sudo apt-get -y install software-center; \
sudo apt-get -y install aptitude;
```

#### Package installs ####
* firefox
* terminator
* filezilla
* Graphical Disk Map
* KeePass2
* Pithos
* System Load Indicator
* Remote Desktop Viewer
* install Wallch  # screen shot desktop
* htop nice top console tool
* vim, nicer vi editor

TODO: need a script
```script
sudo apt-get -y install firefox firefox-locale-en \
    terminator \
    filezilla \
    k4dirstat \
    keepass2 keepass2-doc \
    pithos \
    indicator-multiload \
    remmina remmina-common remmina-plugin-rdp remmina-plugin-vnc \
    wallch \
    htop \
    vim
```

#### Install build tools ####
```script
sudo apt-get install curl corkscrew build-essential libssl-dev zlib1g-dev libxml2-dev libxslt-dev git -y
```

#### Install go lang ####
```script
cd /tmp
# get download from https://golang.org/dl/
if [ "$(uname -s)" = "Darwin" ] ; then
    VERSION=1.6
    ARCH=amd64
    OS=darwin
else
    VERSION=1.6
    ARCH=amd64
    OS=linux
fi
# https://golang.org/dl/
wget "https://storage.googleapis.com/golang/go${VERSION}.${OS}-${ARCH}.tar.gz"

sudo bash -c "tar -C /usr/local -xzf /tmp/go$VERSION.$OS-$ARCH.tar.gz"

[ ! -f $HOME/.goenv.sh ] && echo '. $HOME/.goenv.sh' >> $HOME/.bashrc
echo 'export GOPATH=$HOME/go' >> $HOME/.goenv.sh
echo 'export PATH=$PATH:$GOPATH/bin:/usr/local/go/bin' >> $HOME/.goenv.sh
chmod +x $HOME/.goenv.sh
. $HOME/.goenv.sh
mkdir "$GOPATH/src" "$GOPATH/bin"
chmod 777 $GOPATH
# in case you get error: server certificate verification failed. CAfile
# use : export GIT_SSL_NO_VERIFY=1
go get  github.com/mitchellh/gox \
            github.com/golang/lint/golint \
            github.com/mattn/goveralls \
            golang.org/x/tools/cover \
            github.com/aktau/github-release
# having problems with this one
# go install golang.org/x/tools/cmd/godoc: open /usr/local/go/bin/godoc: permission denied
# installed as root to work around issues.
go get golang.org/x/tools/cmd/godoc

```
#### Bootstrap puppet ####
```script
wget --no-check-certificate \
https://raw.githubusercontent.com/forj-oss/maestro/master/puppet/install_puppet.sh -O - \
 | sudo bash
```

#### MyHome directory setup ####
[My Home Instructions](https://github.com/wenlock/myhome/blob/master/README.md)
* copy Keys to ~/.ssh
* Setup proxy script
```script
sudo bash -c 'echo "export PROXY=http://yourproxy:8080" > /etc/profile.d/00_proxy.sh'
sudo bash -c 'curl https://raw.githubusercontent.com/forj-oss/forj-docker/master/bin/scripts/proxy.sh > /etc/profile.d/01_proxy.sh'
sudo chmod +x /etc/profile.d/??_proxy.sh
```
If you use your computer between work and home, and you don't need a proxy at home, you can optionally turn on / off proxy with contents of 00_proxy.sh being (where 10.0.1.* is your home subet, set this to what yours is):
```script
if [ ! "$(facter ipaddress|grep '10.0.1.*'|wc -l)" -gt 0 ];then
    export PROXY=http://web-proxy.corp.hp.com:8080
fi
```

* secure keys
```script
mkdir ~/.ssh
chmod 700 ~/.ssh
chmod 600 ~/.ssh/*
eval $(ssh-agent); ssh-add ~/.ssh/gerrit_keys
```
* pull myhome repo
```script
cd ~ ; \
git init ; \
git remote add origin git@github.com:wenlock/myhome.git ; \
git pull origin master ; \
git reset --hard origin/master;
```

#### git shortcuts ####
The next script I use to automate and simplify some of the usage of git on my local system at the command prompt.  After installation you'll get a script called ```~/.gitenv``` which will be sourced as a set of funcitons and aliases from the ```~/.profile``` shell environment.  Use ```git-help``` to get started on usage / setup.

```script
bash ~/opt/bin/setup_git_alias.sh
```

#### setup zsh ####
Love this shell over bash for productivity.  For automation bash is still the cross platform winner in my mind.
```script
sudo -i apt-get install zsh -y
wget --no-check-certificate http://install.ohmyz.sh -O - | ZSH=~/.oh-my-zsh sh
chsh -s /bin/zsh
```

#### give your command prompt some bling ####
I like knowing when a file on my git repo has changed, and when things are as they should be.

```script
[ ! -z "$HTTP_PROXY" ] && PIP_PROXY="--proxy $HTTP_PROXY"
sudo -i pip install --user git+git://github.com/Lokaltog/powerline $PIP_PROXY
cd /tmp
wget https://github.com/Lokaltog/powerline/raw/develop/font/PowerlineSymbols.otf https://github.com/Lokaltog/powerline/raw/develop/font/10-powerline-symbols.conf
sudo mv PowerlineSymbols.otf /usr/share/fonts/
sudo fc-cache -vf
sudo mv 10-powerline-symbols.conf /etc/fonts/conf.d/
[ ! -d ~/.fonts ] && mkdir -p ~/.fonts
cd ~/.fonts
wget https://github.com/powerline/powerline/raw/develop/font/PowerlineSymbols.otf
fc-cache -vf ~/.fonts
wget https://github.com/powerline/powerline/raw/develop/font/10-powerline-symbols.conf
```
* Restart terminator
* ```mkdir ~/git;git pull origin master; git reset --hard origin/master;. ./.bashrc```
* reboot the desktop ```sudo reboot```

#### key bindings setup ####
```script
sudo -i apt-get -y install xdotool
https://github.com/dnschneid/crouton/wiki#custom-keys-bindings-via-commands
https://help.ubuntu.com/community/KeyboardShortcuts
sudo -i apt-get -y install xbindkeys xbindkeys-config xvkbd -y
xbindkeys --defaults > /home/$(whoami)/.xbindkeysrc
```
* to change things around
```script
xbindkeys
xbindkeys-config
```
#### Install google drive sync ####
This requires a 15-20$ subscription, but very usefull.
```script
sudo -i bash -c 'echo deb http://apt.insynchq.com/ubuntu/ trusty non-free contrib>> /etc/apt/sources.list'
wget -qO - https://d2t3ff60b2tol4.cloudfront.net/services@insynchq.com.gpg.key \
| sudo -i apt-key add -
sudo -i apt-get update
sudo -i apt-get -y install insync
```

#### install nodejs ####
```script
sudo -i add-apt-repository ppa:chris-lea/node.js
sudo -i apt-get update
sudo -i apt-get -y install nodejs
```

#### install atom ####
```script
sudo -i add-apt-repository ppa:webupd8team/atom
sudo -i apt-get update
sudo -i apt-get -y install atom
```
* Install some [atom plugins](atom-plugins.md)
```script
apm install --packages-file ~/opt/conf/apm_packages.config
```

#### install wine, with wow ####
```script
sudo -i apt-add-repository ppa:foresto/winepatched
sudo -i apt-get update
sudo -i apt-get -y install wine1.7
```
* get mono installed
```wine notepad```

* sound config for wow setting Config.wtf
```
SET Sound_SoundOutputSystem "1"
# SET SoundBufferSize 50-250
# SET SoundOutputSystem "1"
SET Sound_SoundBufferSize "150"
# remove /etc/asound* and ~/.asound*
# padsp winecfg, use OSS
# wine reg add "HKCU\Software\Blizzard Entertainment\Blizzard Downloader" /v "Disable Peer-to-Peer" /t REG_DWORD /d "1" /f
# ui corruption
Set UIFaster "2"
# better frame rates
SET ffxDeath "0"
SET ffxGlow "0"
# stutters, /etc/X11/xorg.conf
Option "UseFastTLS" "2"
```

#### install myrooms ####
TODO: fix instructions, not working.

```script
cd ~/Downloads
wget https://www.myroom.hp.com/downloadfiles/hpmyroom_v10.0.0.0210_amd64.deb
sudo -i dpkg -i ./hpmyroom_v10.0.0.0210_amd64.deb
```

#### pidgin installation ####
* Install pidgin from software center
  Choose more , and select additional packages
* compile sipe
```script
sudo -i apt-get install libgstreamer0.10-dev libnice-dev libpurple-dev libnss3-dev libglib2.0-dev checkinstall intltool -y
mkdir -p ~/Downloads/sipe
cd ~/Downloads/sipe
rm -rf pidgin-sipe-1.18.4
wget http://sourceforge.net/projects/sipe/files/sipe/pidgin-sipe-1.18.4/pidgin-sipe-1.18.4.tar.gz
tar -xzvf pidgin-sipe-1.18.4.tar.gz
cd pidgin-sipe-1.18.4
./configure --with-vv --prefix=/usr
make
sudo -i checkinstall -D make install
#verify the package install
dpkg -s pidgin-sipe
# or install without package
sudo -i make install
```

#### install dig ####
```script
sudo -i apt-get -y install dnsutils
```

#### Installing extra certificates ####
Installing self signed certificates from sites you trust for chrome:
see http://blog.avirtualhome.com/adding-ssl-certificates-to-google-chrome-linux-ubuntu/

1. setup some tools
```script
sudo -i apt-get -y install libnss3-tools
```

2. install common certs
```script
cd ~/Downloads
curl -k -o "cacert-root.crt"   "http://www.cacert.org/certs/root.crt"
curl -k -o "cacert-class3.crt" "http://www.cacert.org/certs/class3.crt"
certutil -d sql:$HOME/.pki/nssdb -A -t TC -n "CAcert.org" -i cacert-root.crt
certutil -d sql:$HOME/.pki/nssdb -A -t TC -n "CAcert.org Class 3" -i cacert-class3.crt
```

3. run script described in "Using my little script" of above link
```bash ~/opt/bin/import-cert.sh hostname portnum```

#### install hipchat ####
```script
sudo bash -c 'echo "deb http://downloads.hipchat.com/linux/apt stable main" > \
  /etc/apt/sources.list.d/atlassian-hipchat.list'
wget -O - https://www.hipchat.com/keys/hipchat-linux.key | apt-key add -
sudo -i apt-get update
sudo -i apt-get -y install hipchat
```

#### install java ####
```script
sudo -i apt-get install default-jre -y
sudo -i apt-get install default-jdk -y
```

#### install oracle java ####

Download http://www.java.com/en/download/manual.jsp
  http://javadl.sun.com/webapps/download/AutoDL?BundleId=109698


```script
cd ~/opt/bin
tar -zxvf ~/Downloads/jre-8u60-linux-x64.tar.gz
cat > $HOME/.ora.java.env << ORA
export JAVA_HOME=\$HOME/opt/bin/jre1.8.0_60
export JAVA_BIN=\$JAVA_HOME/bin
export PATH=\$JAVA_BIN:\$PATH
ORA
chmod +x $HOME/.ora.java.env
echo '. $HOME/.ora.java.env' >> $HOME/.bashrc

sudo -s
cd /usr/lib/mozilla/plugins
ln -s $JAVA_BIN/jre1.8.0_60/lib/amd64/libnpjp2.so
```

#### setup latest version of seahorse for gnome keyring ####
```script
sudo -i apt-get -y install seahorse-nautilus
killall nautilus && nautilus &
```

#### setup shellcheck ####
```script
sudo -i apt-get install -y cabal-install
cabal update
cabal install shellcheck
```
http://goo.gl/vXxfpt

#### Install Google Chrome ####
* Visit google.com/chrome with firefox, download and autostart chrome installation
```script
cd /etc/puppet/modules
sudo -i puppet module install juniorsysadmin-chromerepo
sudo -i puppet apply --verbose --debug --modulepath=/etc/puppet/modules -e "include chromerepo"
sudo -i apt-get install libxss1 libappindicator1 libindicator7
sudo -i apt-get -y install google-chrome-stable
```

#### Configure Google Chrome App Launcher ####
   https://chrome.google.com/webstore/launcher
   search in app start for app launcher, and lock to tool bar.

#### chrombook kernel headers installation ####
On chromebooks, if you plan to use virtual box, there is a nice [guide that explains](https://github.com/divx118/crouton-packages) how to install kernel headers.  Here are some shortcuts that cover the installation in the guide.

* disable kernel headers, from cros shell in chromeos run:
```script
cd ~/Downloads
wget https://raw.githubusercontent.com/divx118/crouton-packages/master/change-kernel-flags
sudo sh ~/Downloads/change-kernel-flags
sudo reboot
```
* login to chroot ubuntu and run the kernel headers install script:
```script
cd /tmp
wget https://raw.githubusercontent.com/divx118/crouton-packages/master/setup-headers.sh
sudo sh setup-headers.sh
```
* enable [vmx](https://gist.github.com/DennisLfromGA/cd3455530cec2a5a1ef4) as well on chromebooks for virtual box
```script
cd ~/Downloads
wget https://gist.githubusercontent.com/DennisLfromGA/cd3455530cec2a5a1ef4/raw/5d311d6eca3b847878c22927fab7d37a32482490/enable-vmx.sh
sh ./enable-vmx.sh
egrep '(vmx|svm)' /proc/cpuinfo  # should return a vmx flag enabled.
```

#### Install virtualbox 5 (experimental) ####
# from https://www.virtualbox.org/wiki/Testbuilds
```script
cd /tmp
curl https://www.virtualbox.org/download/testcase/VirtualBox-5.0.1-101939-Linux_amd64.run > vbox.sh
chmod +x ./vbox.sh
sudo ./vbox.sh
sudo adduser $(whoami) vboxusers
```
NOTE: vbox may not install if you don't have a proper /etc/rc.local, the script has some errors at line 18 that need to be fixed in the if statement.

#### Virtualbox troubleshooting

##### Win 8.1/2012 images don't boot for installation

* see: https://www.virtualbox.org/ticket/11841
* ```vboxmanage setextradata "WIN8_COE" VBoxInternal/CPUM/CMPXCHG16B 1```


#### Install vagrant ####
* install vagrant, latest is available [here](https://www.vagrantup.com/downloads.html).
```script
cd ~/Downloads
wget https://dl.bintray.com/mitchellh/vagrant/vagrant_1.7.4_x86_64.deb -O ./vagrant.dep
sudo dpkg -i ./vagrant.dep
```

#### Install forj-docker ####
* setup bundler
```script
sudo apt-get install ruby1.9.1-dev
sudo gem install bundler --no-rdoc --no-ri
```
* setup forj-docker
```script
mkdir -p ~/git/forj-oss
cd ~/git/forj-oss
# this assumes ~/.ssh/config has review alias
git-clone review:forj-oss/forj-docker
cd forj-docker
ruby -S bundle install --gemfile Gemfile
```
* build a dev image for running docker
```script
export DOCKER_VERSION=latest
rake rundev
# you should use `rake 'configure[vagrant]' for virtualbox installation of docker
rake 'configure[bare]'
rake provision
reboot
rake provision
```

#### private vpn setup####
No one but hp employees will be able to use the pip module.
```script
sudo apt-get update
sudo apt-get install openvpn network-manager-openvpn-gnome

https://hpedia.hp.com?SSL+OpenVPN#Python_Script
cd ~/Downloads
sudo pip install Hpvpn-1.0.1.zip
sudo hpvpn
```

#### chage reboot policy in ####
 /usr/share/polkit-1/actions/org.freedesktop.consolekit.policy

#### Setup sshd server ####
To connect to your desktop with ssh
```script
sudo -i apt-get update
sudo -i apt-get install -y openssh-server
sudo -i mkdir -p /var/run/sshd
```
Specific to chromeos you need to enable the [service to run](https://github.com/dnschneid/crouton/wiki/Running-servers-in-crouton):
```script
sudo apt-get -y install iptables
sudo /sbin/iptables -I INPUT -p tcp --dport 22 -j ACCEPT
```
Edit /etc/rc.local with :
```script
mkdir -p -m0755 /var/run/sshd
/usr/sbin/sshd
```

#### installing python ####
```script
sudo -i apt-get update -y ; \
sudo apt-get install python-dev build-essential libssl-dev zlib1g-dev libxml2-dev libxslt-dev git curl -y
```

#### installing pip ####
```script
cd /temp ; \
curl https://bitbucket.org/pypa/setuptools/raw/39f7ef5ef22183f3eba9e05a46068e1d9fd877b0/ez_setup.py --insecure > ez_setup.py ; \
curl https://raw.githubusercontent.com/pypa/pip/develop/contrib/get-pip.py --insecure > get-pip.py ;\
sudo -i python /tmp/ez_setup.py ;\
sudo -i python /tmp/get-pip.py
```

#### installing openstack clients ####
```script
echo 'python-barbicanclient
python-ceilometerclient
python-cinderclient
python-glanceclient
python-heatclient
python-ironicclient
python-keystoneclient
python-magnetodbclient
python-manilaclient
python-marconiclient
python-melangeclient
python-neutronclient
python-novaclient
python-openstackclient
python-quantumclient
python-saharaclient
python-solumclient
python-swiftclient
python-troveclient' | sudo -i xargs -i pip install {}
```

#### install ruby ####
```script
sudo apt-get -y install git-core curl zlib1g-dev build-essential libssl-dev libreadline-dev libyaml-dev libsqlite3-dev sqlite3 libxml2-dev libxslt1-dev libcurl4-openssl-dev python-software-properties libffi-dev
sudo apt-get install libgdbm-dev libncurses5-dev automake libtool bison libffi-dev
curl -L https://get.rvm.io | bash -s stable
source ~/.rvm/scripts/rvm
rvm install 2.2.3
rvm use 2.2.3 --default
ruby -v
echo "gem: --no-ri --no-rdoc" > ~/.gemrc
gem install bundler
```
