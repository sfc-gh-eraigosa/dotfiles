Atom.io provides a hackable text editor that is super light weight on memory <200MB and it's usable.

* [Install the puppet, node tools](puppet27.md)
* ```sudo apt-get install build-essential git libgnome-keyring-dev```
* [Atom build process](https://github.com/atom/atom/blob/master/docs/build-instructions/linux.md#instructions):
```sh
  git clone https://github.com/atom/atom
  cd atom
  script/build # Creates application at $TMPDIR/atom-build/Atom
  sudo script/grunt install # Installs command to /usr/local/bin/atom
  script/grunt mkdeb # Generates a .deb package at $TMPDIR/atom-build, e.g. /tmp/atom-build
```
