## Overview
This procedure can be used to split a git repository into smaller git repositories. In this case, the config project is looking to solve the problem of having monolythic puppet modules repository so that unit testing can be more component based.

## References
* [openstack migration](https://review.openstack.org/#/c/99990/9)
* [installing git subtree - engineeredweb](http://engineeredweb.com/blog/how-to-install-git-subtree/)
* [installing git subtree - lostechies](http://lostechies.com/johnteague/2014/04/04/using-git-subtrees-to-split-a-repository/)
* [installing git subtree - ubuntu 12.04](http://blogs.atlassian.com/2013/05/alternatives-to-git-submodule-git-subtree/)
* [installing git subtree - ubuntu 13.04](http://cogumbreiro.blogspot.com/2013/05/how-to-install-git-subtree-in-ubuntu.html)

## Install subtree
Do the following if git subtree do not return the subtree help.
```script
mkdir -p ~/git
cd git
git clone https://github.com/git/git
cd git/contrib/subtree
make all
sudo install -m 755 git-subtree /usr/lib/git-core
```

## Create subtree repo using the old repo

Setup an empty clone of the old repo
```script
cd ~/git
mkdir split
cd split
git clone https://giturl/oldrepo
cd oldrepo
```

Create a subtree for the folder path that will move to the new repo.
```script
git subtree split --prefix=folderpath/tofolder/tosplit -b target_branch_name
git checkout target_branch_name
```

## Push the branch into an empty repo

Initialize a new repo to accept the changes from the sub folder.
```script
cd ~/git/split
mkdir newrepo
cd newrepo
git init --bare
```

Clone the old repo and update the new repo.
```script
cd ~/git/split
git clone giturl/oldrepo
cd ~/git/split/oldrepo
git push ~/git/split/newrepo target_branch_name:master
```

## publish the empty repo to a new repo location, like github
create a new github project thats emptym, lets call it newrepo
```script
cd ~/git/split
mkdir migrate
cd migrate
git clone file:///$HOME/git/split/newrepo
cd newrepo
git remote add  upstream git@github.com:orgname/newrepo.git
git push upstream master
```
