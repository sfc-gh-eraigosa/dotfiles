myhome
======

Ubuntu Bash Home profile setup, includes vimrc's profiles some git commands

I plan to use this primarly to manage my ux home directory across various
systems where i plan to use git and cli based tools.  I want a nice
editor as well where the backgrounds and sytax highlighting work.

I've been mostly developing puppet modules, so those will likely be
the focus here.

I use a chromebook for most of my UX development so after I get my new
chromebook, I'll be installing cruton on it and adding some stuff to help me there.

alias and commands
==================

gitsave_off  :  Turn off the git commit/push on bash exit
gitsave      :  Turn on git commit/push on bash exit

vimw         :  Open vim with white background
vimg         :  Open vim with green background
vimw_set     :  Set the default to be white background
vimg_set     :  Set the default to be green background

structure
=========
I start off by ignoring everything with .gitignore.   I'm not silly, I don't want everything commited from my home folder.
If I care about saving it, I'm going to add it to the .gitignore file with the following exception.

Here is my exception.   

Tools I use that need to be autosaved when I logout will go in ~/opt.
My rule is that if it's specific to my development workflow, then it belongs under ~/opt
~/opt/bin - saving my shell scripts and such
~/opt/lib - saving any python or ruby help me scripts
~/opt/docs - where i save my howto's and such where im learning new stuff or trying to remind myself how to do stuff

On logout i want .bash_logout to save and commit my changes

my wish list
============
Working .xsession profiles
Working atom profiles scripts to install on chrome
Working docker scripts to setup my docker environment


Contributing to myhome
======================

Want to help me be more productive, wow cool!  

Fork this repo and tell me how :D   I'm willing to try most workflows if it 
makes me a better programmer.   I'd love to see ideas and techniques that
can help me get there.

These are definetly not perfect, please let me know if anything feels
wrong or incomplete.  Especially if I make a security boo boo.


Licensing
=========
myhome is licensed under the Apache License, Version 2.0. See LICENSE for full license text.
