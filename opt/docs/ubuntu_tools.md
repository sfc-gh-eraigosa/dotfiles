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
sudo apt-get -y install apptitude;
```

#### Package installs ####
* firefox
* terminator
* filezilla
* Visit google.com/chrome with firefox, download and autostart chrome installation
TODO: need a script
```script
sudo apt-get -y install 
```

#### Configure Google Chrome App Launcher ####

   run from chrome: chrome://flags/#enable-app-list
   restart chrome
   search in app start for app launcher, and lock to tool bar.

#### Install build tools ####
```script
sudo apt-get install corkscrew build-essential libssl-dev zlib1g-dev libxml2-dev libxslt-dev git -y
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
sudo bash -c 'curl https://raw.githubusercontent.com/forj-oss/forj-docker/master/bin/scripts/proxy.sh > /etc/profile.d/01_proxy.sh`
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
sudo apt-get install zsh -y
wget --no-check-certificate http://install.ohmyz.sh -O - | ZSH=~/.oh-my-zsh sh
chsh -s /bin/zsh
```

#### give your command prompt some bling ####
I like knowing when a file on my git repo has changed, and when things are as they should be.

TODO: Finish doc....

Tools
-----
* [Install Chrome Browser on ubuntu](chrome-browser.md)
* Software Center ```sudo apt-get install software-center```
* Google Talk
* Terminator, terminals work better than xterm with copy / paste (ctrl-shift-c, ctrl-shift-v)
* [Install puppet and some modules using forj-oss/maestro](puppet27.md)
* Setup a bunch of base [packages with puppet](puppet_packages.md)
* [Atom editor](atom.md)
* [Fonts for zsh](powerline-fonts.md)

Investigate
-----------
* PlayOnLinux
* [Setting up virtualization test envs](virtualization.md)
* [Mail](davmail.md)
