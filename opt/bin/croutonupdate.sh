SCRIPT_DIR=$(dirname $0)
SCRIPT_DIR=$(cd $SCRIPT_DIR;pwd)
cd ~/Downloado
[ ! -f ./crouton ] && curl https://goo.gl/fd3zc > ./crouton
sudo bash ./crouton -n trusty -u
