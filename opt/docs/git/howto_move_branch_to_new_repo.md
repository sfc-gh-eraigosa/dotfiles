## Overview
Lets say you want to create a new git repo from a branch from another git repo.
```
<oldrepo>:<oldbranch> -->>  <newrepo>:master
```

## Setup the new empty repo that you want to copy the branch to

```script
mkdir ~/newrepo
cd ~/newrepo
git init
git config --add receive.denyCurrentBranch ignore
```

## Checkout the latest changes from the source prach

```script
cd ~/oldrepo
git checkout oldbranch
git push file://<path to new repo> +oldbranch:master
```

## reset the files on the new repo
```script
cd ~/newrepo
git reset --hard
```
