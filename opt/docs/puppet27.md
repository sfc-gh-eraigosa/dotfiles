Forj maestro module on review.forj.io has some shell scripts that can be used to setup puppet on ubuntu.

```sh
cd ~/git
ssh-add ~/.ssh/<pemfile>
git clone ssh://review.forj.io:29418/forj-oss/maestro
cd maestro/puppet
sudo bash ./install_puppet.sh
sudo bash ./install_modules.sh
cd ../hiera
sudo bash ./hiera.sh
```

You can then install other tools.  First set the PUPPET_MODULES export:
```sh
export PUPPET_MODULES=$HOME/git/maestro/puppet/modules:/etc/puppet/modules
```

* fog libs & hpcloud cli: ```sudo puppet apply --modulepath=$PUPPET_MODULES -e 'include gardener::requirements'```
* Python 2.7 + pip : ```sh
sudo wget https://raw.github.com/pypa/pip/8575e0c16424bcc9866baa0f9f779f1b524fbc20/contrib/get-pip.py -O /var/lib/python-install/get-pip.py
sudo puppet apply --modulepath=$PUPPET_MODULES -e 'include pip::python2'
```
* NodeJS and NPM : ```sudo puppet apply --modulepath=$PUPPET_MODULES -e 'include nodejs_wrap'```
* Gerrit Review workflow tool : ```sudo pip install git-review```
