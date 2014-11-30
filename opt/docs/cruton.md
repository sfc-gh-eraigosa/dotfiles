Crouton Notes
============

I finally got enough [recognition points](http://www.businessinsider.com/whitman-wins-the-love-of-hp-employees-2013-4) to get a new chromebook to hack with.  The chromebook I had before couldn't be hacked on because it was being used by several other family members.  Now that we have a second chromebook, I get to keep the old chromebook to hack on, and the family can use the new chromebook, 
![woot](https://lh6.ggpht.com/-Iv1RvuzpCIDFs9Mx_-LrFaLSjBiBWT6rkMl3QSZYWsFb62RRUzefIwD4xKXGOYPiU-w=w300-rw)!


Prep Work
---------
OK, lets start with storage.   I wanted to have enough storage besides the minimal 16G that comes with the chromebook.  This way I can install Linux software to the external storage, and remove it when I'm feeling I'm not in the most secure location.   I can then walk away with the SD card, and be happy that I left my chromebook in the car, in case it gets stolen.   I hear I can encrypt that too, so I might play with it later to see if I get performance.   For the card, I bought what I thought would be the fastest and most affordable drive.  I found this one at best buy:  [Model: SDSDXS-064G-A46 SKU: 4807118](http://www.bestbuy.com/site/sandisk-extreme-plus-64gb-sdxc-memory-card/4807118.p?id=1218531607004&skuId=4807118&st=Sandisk%2064gb&cp=1&lp=2)

Normally I see those for 200$ so for 70$ and 80MB/sec, I think it's a good deal.  Also, I've seen most ubuntu boxes run well with 50G or so of disk space.   I can also add more storage later with USB drives, but hopefully I won't have to.

![](http://pisces.bbystatic.com/image2/BestBuy_US/images/products/4807/4807118_sa.jpg;canvasHeight=139;canvasWidth=105)
Also, if your planning to use wine, virtualbox or docker in combination with any of these other virtualization tools, I highly recommend upgrading to the 128GB or 256GB drive.

Setting up developer mode
-------------------------
Now I switched the computer from normal mode to developer mode.  Couple of gotchas with this.   
* Your kernel is no longer secure, which means, secure the underlying OS.   I plan to only do opensource stuff here, so if I do other stuff that needs protection, I'll likely use a truecrypt drive or something to protect the private stuff.  To lockdown the root / chronos user accounts, you can issue the command ```chromeos-setdevpasswd```.  From there forward, you'll need a password to issue sudo commands and login with root commands.  Beware, even though your account is locked though, you can still switch into the logged in user account with ctrl-alt-shift.  When locking the computer, make sure to logout and then lock the system.  Comments welcome on this topic.
* At boot up your going to get a funky warning about recovery mode.  I'm not sure what the best approach here is yet, other than keeping everyone away from your chromebook and don't let them touch it.  Thus why I waited to get a second chromebook for this.   I'm also thinking of getting a shell cover for 20$ and just sticking a little warning on the cover, like, DO NOT ENTER, or DONT TOUCH without Permissions.   Something to ward off the kids.   It also means don't be lazy and forget to hit ctrl-d at boot up everytime.   You might end up reseting your os.   
* This brings me to another point, loosing your OS / or more over your data on a chromebook seems easy to do.  Get an SD card to store that stuff, and maybe setup some automation.  I'll talk more about that later, but I'm hoping to use github for this on the unsecure settings.   Lets see how this goes.

Setting up developer mode was very easy.  Here are the steps I took after [reading the debian distro site](https://wiki.debian.org/InstallingDebianOn/HP/Chromebook%2014#Important_Note)

Steps:

1. save any data you want from Downloads.  Most of your data should already be on your google account, so you might have to remember some passwords you didn't write down.  Now's the time to do that if you don't use anything to save things like that.  You might want to tell the others your sharing that chromebook with the same thing, they'll loose there stuff too.  Plus good opportunity to kick them out, hehe.  Make sure the chromebook is fully charged.
2. Power off your chromebook.   
3. As you power on your chromebook, hold the esc+refresh button.  A message about no os found will apear.
4. On the no os found message, hit ctrl-d, then follow and read the prompts.
Setting up developer mode takes about 15minutes, while it downloads stuff and configures your box.  I did mine while it was not plugged in, but later it occurred to me that I might want to have done that on a plug and fully charged.

Once your in developer mode, login and get back onto your account.  You might also need to join a network.  You'll need it for the next step for sure.

Change Root Setup
-----------------
I'm using [crouton](https://github.com/dnschneid/crouton), which is a chroot OS that utilizes the underlying chrome OS linux kernel to execute in.   I expect to get most features of ubuntu this way while continuing to utilize my chrome OS for the features it's good at.   I'm also hoping it gives me a good user experience when doing development outside of the chromeOS UI.   For the [crouton](https://github.com/dnschneid/crouton) flavor, I'm trying awesome UI with ubuntu 12.04.  I might experiment with others once I get usto all the [crouton](https://github.com/dnschneid/crouton) chroot options.   I read we can do many chroots, so I'm hoping to try this as well for switching between precise and trusty versions.  Start with the github project wiki instructions [here](https://github.com/dnschneid/crouton/wiki/awesome)

If you follow those instructions exactly, you should end up with awesome UI plus ubuntu 12.04.  Give yourself 1-2 hours for the chroot setup to complete.  

UPDATE: I switched away from awesome UI to the unity UI.  This was alot more user friendly and seemed easier to setup.

I'll be doing this again with some options for installing my chroot to usb.  Such as :
```sh
# this is recommended method for installing all chroots
ctrl+alt+t

>shell

sudo sh -e ./crouton -t unity
```

You can list other distributions with the command : ```sudo sh -e ./crouton -t list```
At then end of the install, you'll see something like:

You can flip through your running chroot desktops and Chromium OS by hitting
Ctrl+Alt+Shift+Back and Ctrl+Alt+Shift+Forward.

You can also use ctrl+alt+back and forward as well to transition between ux prompt and chromos, but when you do this while a window manager is running, it will stop.

Unmounting /usr/local/chroots/precise...
Done! You can enter the chroot using enter-chroot.

Unity setup
-----------
After a bit of playing with awesome, I came to the conclusion that I don't want to spend alot of time hacking on my user interface.  For this reason, I switched to a more simple ready to go user interface that had some familliar navigation with basic customization for things like the desktop.  To enter a chroot, you should do these steps (we use unity as the example here):

```sh
ctrl+alt+t

>shell

sudo sh -e ./crouton -t unity -n unity
startunity
```
I noticed that the unity user interface tends to start best when you kick it off from chros shell.  You should also be running as the user chronos.  If for some reason you get stuck at the root account, you can switch to chronos with the command:
```sh
su - chronos
```

Customizing my shell
-----------------------
Once I'm booted on my new chroot, I customize my desktop by using this projects myhome files.
I change to using git at this point with github.com/wenlock/myhome
sudo apt-get install git vim corkscrew

```sh
mkdir ~/.ssh
chmod 700 ~/.ssh
echo > '<your privatekey>' > ~/.ssh/gerrit_keys
echo > '<your public key>' > ~/.ssh/gerrit_keys.pub
chmod 600 ~/.ssh/*
eval $(ssh-agent); ssh-add ~/.ssh/gerrit_keys

cd ~
git init
git remote add origin git@github.com:wenlock/myhome.git
git pull origin master
git reset --hard
```


Move chroots to SD Card
-----------------------
[Tom Wolf posted](http://tomwwolf.com/chromebook-14-compedium/chromebook-crouton-cookbook/) a nice tip to setup the /usr/local/chroots folder onto an SD card.
This gives you a way to install lots more stuff on your SD card, than what disk space is available on your internal drive.  The performance for this was not that bad with a 80MB/sec SD card.

* Format the sd card with ext2 filesystem

```sh
sudo fdisk -l # point here is to make sure the sd card is /dev/sdb device
sudo umount /dev/sdb1
sudo mke2fs /dev/sdb
```

* Eject the device and shutdown the chromebook.  When powered off, you can re-insert the device and start the chromebook.  This insures that you have the drive properly mounted with the USB Drive name.

* Move any chroots you want to keep to the USB Drive

```sh
sudo mkdir -p '/media/removable/USB Drive/chroots'
sudo edit-chroot -m '/media/removable/USB Drive/chroots/' precise
```

* Link the chroot folder to the SD Drive

```sh
cd /usr/local
sudo mv  chroots/ '/media/removable/USB Drive/'
sudo ln -s '/media/removable/USB Drive/chroots/' chroots
```

Tips
---------
* When switching off [crouton](https://github.com/dnschneid/crouton), I found that closing the lid on hp chromebook 14 would cause the system to freeze.  To avoid that try switching back to chromeUI (ctrl-shift-forward), then close the lid.  FIXUPDATE: This seems to now be fixed with latest version of crouton and trusty image ( -t unity -r trusty ).
* When establishing a vpn with openvpn, try setting up a persistent tunel before hand, [found it on this issue](https://github.com/dnschneid/crouton/issues/375): ```openvpn --mktun --dev tun0```.  I also setup an alias for this, tunnelp.
* If you leave the system alone while in ubuntu, your power safe sets your screen dimmed.  This can't be undone.  To avoid this, turn off power save mode: ```sudo initctl stop powerd```
* Setting up atom on [crouton, works](atom.md).
* [Puppet setup using 2.7](puppet27.md).
* Perform some [backups with these instructions](https://github.com/dnschneid/crouton#a-backup-a-day-keeps-the-price-gouging-data-restoration-services-away).
Wish List
---------
Things I need to get documented on how to do or find out if they work.

* Fix my magic mouse scroll wheel.
* Sharing session with hp rooms (www.myrooms.hp.com works like a charm)
* Does lync work? (yes, with pidgin-sipe plugin, note, best when compiled from source, TBD)
* Syncronizing my local drive with the google drive on chromeOS to sync changes  (potential workaround insync, but this is a pay solution and doesn't seem to reliably start on the UI).
* Copy paste from chromOS to chroot OS (workaround in place is to use google keep).
* Steps for compiling a customer kernel for running docker or virtualbox  (now working TBD, post an update for this, this is covered on crouton wiki).
* Setting up wine with wow.

Contribute
==========
Tell me about your experience with crouton and this awesome work from those folks.   Any gotchas or problems it you solved?
