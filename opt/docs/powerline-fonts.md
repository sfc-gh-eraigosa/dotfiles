Powerline fonts are needed for some of the zsh themes, so here are some steps for setting it up.
Lets use pip, it works.
Also see [this post on ask ubuntu.](http://askubuntu.com/questions/283908/how-can-i-install-and-use-powerline-plugin)

myhome already sets up most of these steps for per user, but
you'll need to do some additional steps as listed in the link for system wide setup.  myhome shouldn't do that.

Installation will need to have at least this step to work with myhome:

* pip install --user git+git://github.com/Lokaltog/powerline
