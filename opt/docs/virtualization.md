For virtualization like vagrant, docker, and virtual box.

```sh
sudo puppet module install gini-virtualbox
sudo puppet module install shr3kst3r-vagrant
sudo puppet module install garethr-docker

sudo puppet apply --modulepath /etc/puppet/modules -e "
include virtualbox
include vagrant
class{'docker': manage_kernel=>false}
"
```
