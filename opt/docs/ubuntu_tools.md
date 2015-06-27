Ok, time to update these steps.  I have a long list of notes I've taken on setting up and running my desktop.
I'll list them here and try to order them.

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
mkdir /tmp
cd /tmp
# get download from https://golang.org/dl/
VERSION=1.4.2
ARCH=amd64
OS=linux
wget "https://storage.googleapis.com/golang/go${VERSION}.${OS}-${ARCH}.tar.gz"

sudo bash -c "tar -C /usr/local -xzf /tmp/go$VERSION.$OS-$ARCH.tar.gz"
sudo bash -c "echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile"
[ ! -d $HOME/go ] && mkdir -p $HOME/go
[ ! -f $HOME/.goenv.sh ] && echo '. $HOME/.goenv.sh' >> $HOME/.bashrc
echo 'export GOROOT=$HOME/go' > $HOME/.goenv.sh
echo 'export PATH=$PATH:$GOROOT/bin' >> $HOME/.goenv.sh

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

   run from chrome: chrome://flags/#enable-app-list
   restart chrome
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

#### Install virtualbox ####
```script
sudo bash -c 'echo deb http://download.virtualbox.org/virtualbox/debian trusty contrib >> /etc/apt/sources.list'
cd ~/Downloads
wget http://download.virtualbox.org/virtualbox/debian/oracle_vbox.asc
sudo apt-key add oracle_vbox.asc
sudo apt-get update
sudo apt-get -y install virtualbox-4.3
sudo adduser $(whoami) vboxusers
```
NOTE: vbox may not install if you don't have a proper /etc/rc.local, the script has some errors at line 18 that need to be fixed in the if statement.

#### Install vagrant ####
* install vagrant, latest is available [here](https://www.vagrantup.com/downloads.html).
```script
cd ~/Downloads
wget https://dl.bintray.com/mitchellh/vagrant/vagrant_1.7.2_x86_64.deb -O ./vagrant.dep
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
rake rundev
rake 'configure[bare]'
rake dev
reboot
rake dev
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
 
