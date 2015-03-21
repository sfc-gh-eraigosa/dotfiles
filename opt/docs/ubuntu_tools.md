Ok, time to update these steps.  I have a long list of notes I've taken on setting up and running my desktop.
I'll list them here and try to order them.

Ubuntu Post Setup
-----------------
These instructions are mainly for my setup but might apply to others who are doing the same.  Feel free to borrow as needed.

I've mainly been using these steps on Ubuntu 14.04 standalone and versions I install from cruton on ChromeOS

#### HP Elitebook 8500 special setup ####
Setup nvidia drivers and bios options.  Only need to perform for Elitebook 8500 laptops.

1. edit /etc/defaults/grub, add line after quiet splash, nouveau.modeset=0
2. update grub: ```sudo update-grub```

Configure the video drivers

1. open system settings -> drivers -> software & update -> Additional Drivers
2. use driver: NVIDIDA binary driver - version 331.113 from nvidia-331 (proprietary, tested)
3. ```sudo reboot```

Configure the system bios by disable fastboot, or use bios defaults

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
