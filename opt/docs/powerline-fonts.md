Powerline fonts are needed for some of the zsh themes, so here are some steps for setting it up.
Lets use pip, it works.
Also see [this post on ask ubuntu.](http://askubuntu.com/questions/283908/how-can-i-install-and-use-powerline-plugin)

myhome already sets up most of these steps for per user, but
you'll need to do some additional steps as listed in the link for system wide setup.  myhome shouldn't do that.

Installation will need to have at least this step to work with myhome:

* pip install --user git+git://github.com/Lokaltog/powerline
* Setup the other fonts:
```sh
if [ -d ~/git/powerline-fonts ] ; then
  cd ~/git/powerline-fonts
  find . -name \*.otf|xargs -i cp "{}" ~/.fonts/
  fc-cache -vf ~/.fonts
fi
```

In addition, if you use chromeos and have put your system into developer mode, you can also enable powerline fonts for ChomeOS Shell app.   This [issues talks](https://code.google.com/p/chromium/issues/detail?id=320364) about the details on how to do it, but here are the steps that worked for me:

Open shell, ctrl-alt-t, type > shell
```sh
sudo mkdir -p /usr/local/share/fonts
sudo wget -P /usr/local/share/fonts https://raw.github.com/Lokaltog/powerline/develop/font/PowerlineSymbols.otf
mkdir -p /tmp/test/
sudo mount --bind /home/chronos/ /tmp/test/
cd /tmp/test/user
cat > .font.conf << FONTS
chronos@localhost ~ $ cat .fonts.conf 
<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "fonts.dtd">
<fontconfig>
        <dir>/usr/local/share/fonts</dir>
</fontconfig>
FONTS
```

Restart chromeOS
