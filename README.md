dotfiles
----
[![Docker Image CI](https://github.com/sfc-gh-eraigosa/dotfiles/actions/workflows/docker-image.yml/badge.svg)](https://github.com/sfc-gh-eraigosa/dotfiles/actions/workflows/docker-image.yml)

Ubuntu Bash/zsh Home profile setup, includes vimrc's profiles some git commands

I plan to use this primarly to manage my ux home directory across various
systems where i plan to use git and cli based tools.  I want a nice
editor as well where the backgrounds and sytax highlighting work.

install
----
1. You'll need homebrew, git and vim to setup your home folder.  Make sure to install git and vim for your server, or ask your administrator to do this.
  
  ```sh
   sudo apt-get install git vim corkscrew
   /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  ```
  NOTE: corkscrew installation is used for proxies, and is optional, but not needed here.
2. Setup your home directory with your github account ssh keys and anything else you might need. Please note that the `.bashrc`, etc files will be replaces with soft links.
3. Fork the dotfiles project to your account.  While still logged into your account, now it's time to setup your home directory. You can clone the repo and install it as follows:

  ```sh
    cd ~
    git clone git@github.com:<youruser>/dotfiles.git
    ./dotfiles/install.sh
  ```
4. What you should expect is for files from the `./dotfiles` folder to now be lined to your home directory.  Here is a breakdown of what's linked:
  - All files in `./dotfiles/opt/profiles/*` are now linked to home
  - The linke `./Brewfile` will install a bunch of other tools
  - `./dotfiles/opt` -> `~/opt`
  - `~/opt/bin` will be in the `PATH`
  - default shell is `zsh`
5.  If you plan to use [zsh](opt/docs/zsh_andtools.md) as your default shell, take the time to setup [powerline fonts](opt/docs/powerline-fonts.md).

  ```sh
  git clone https://github.com/powerline/powerline ./powerline
  pip install --user ./powerline/
  mkdir -p ~/.fonts
  cd ~/git/powerline-fonts/
  find . -name \*.otf|xargs -i cp "{}" ~/.fonts/
  find . -name \*.otf|xargs -i cp "{}" ~/.fonts/
  fc-cache -vf ~/.fonts
  ```
6. Set your default shell to `zsh`
  ```
  sudo chsh -s $(which zsh) $(whoami)
  ```
  Exit and restart the shell.

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


Contributing
----

Want to help me be more productive, wow cool!  

Fork this repo and tell me how :D   I'm willing to try most workflows if it 
makes me a better programmer.   I'd love to see ideas and techniques that
can help me get there.

These are definetly not perfect, please let me know if anything feels
wrong or incomplete.  Especially if I make a security boo boo.


Licensing
----
dotfiles is licensed under the Apache License, Version 2.0. See LICENSE for full license text.

Testing
