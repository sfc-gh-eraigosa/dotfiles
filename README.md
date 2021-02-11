dotfiles
----
![Docker Image CI](https://github.com/wenlock/dotfiles/workflows/Docker%20Image%20CI/badge.svg)

Ubuntu Bash/zsh Home profile setup, includes vimrc's profiles some git commands

I plan to use this primarly to manage my ux home directory across various
systems where i plan to use git and cli based tools.  I want a nice
editor as well where the backgrounds and sytax highlighting work.

I've been mostly developing puppet modules, so those will likely be
the focus here.

I use a chromebook for most of my UX development so after I get my new
chromebook, I'll be installing cruton on it and adding some stuff to help me there.

install
----
1. You'll need git and vim to setup your home folder.  Make sure to install git and vim for your server, or ask your administrator to do this.
  
  ```sh
   sudo apt-get install git vim corkscrew
  ```
  NOTE: corkscrew installation is used for proxies, and is optional, but recommended.
2. Setup your home directory with your github account ssh keys.  Lets assume these are called git_keys (private key) and git_keys.pub (public key).

  ```sh
    mkdir ~/.ssh
    chmod 700 ~/.ssh
    echo > '<your privatekey>' > ~/.ssh/git_keys
    echo > '<your public key>' > ~/.ssh/git_keys.pub
    chmod 600 ~/.ssh/*
    eval $(ssh-agent); ssh-add ~/.ssh/git_keys
  ```
3. Fork the dotfiles project to your account.  While still logged into your account, now it's time to setup your home directory, for me as user wenlock, this looks like:

  ```sh
    cd ~
    git init
    git remote add origin https://github.com/wenlock/dotfiles
    git pull origin main
    git reset --hard
  ```
4.  You should now be using the basic dotfiles project. Setup the defaults:

  ```sh
    ~/install.sh
  ```

5.  If you plan to use [zsh](opt/docs/zsh_andtools.md) as your default shell, take the time to setup [powerline fonts](opt/docs/powerline-fonts.md).

  ```sh
    pip install --user git+git://github.com/Lokaltog/powerline
    if [ -d ~/git/powerline-fonts ] ; then
      cd ~/git/powerline-fonts
      find . -name \*.otf|xargs -i cp "{}" ~/.fonts/
      fc-cache -vf ~/.fonts
    fi
  ```

alias and commands
----
```

gitsave_off        :  Turn off the git commit/push on bash exit
gitsave            :  Turn on git commit/push on bash exit

vimw               :  Open vim with white background
vimg               :  Open vim with green background
vimw_set           :  Set the default to be white background
vimg_set           :  Set the default to be green background
vimprompt {on|off} : requires that you have powerline fonts installed with zsh, a way to togle between vim prompt and zsh prompt. Default is off.
```

structure
----
I start off by ignoring everything with .gitignore.   I'm not silly, I don't want everything commited from my home folder.
If I care about saving it, I'm going to add it to the .gitignore file with the following exception.

Here is my exception.   

Tools I use that need to be autosaved when I logout will go in ~/opt.
My rule is that if it's specific to my development workflow, then it belongs
```
under ~/opt
~/opt/bin - saving my shell scripts and such
~/opt/lib - saving any python or ruby help me scripts
~/opt/docs - where i save my howto's and such where im learning new stuff or trying to remind myself how to do stuff
             checkout this section for tips on installing and setting up zsh for a nice git prompt.
```
On logout i want .bash_logout to save and commit my changes

my wish list
----
* Working docker scripts to setup my docker environment
* a way to toggle through zsh prompts and set an active prompt, the default should be whats in .zshrc
* Working docker scripts to setup my docker environment


Contributing to myhome
----

Want to help me be more productive, wow cool!  

Fork this repo and tell me how :D   I'm willing to try most workflows if it 
makes me a better programmer.   I'd love to see ideas and techniques that
can help me get there.

These are definetly not perfect, please let me know if anything feels
wrong or incomplete.  Especially if I make a security boo boo.


Licensing
----
myhome is licensed under the Apache License, Version 2.0. See LICENSE for full license text.

Testing
