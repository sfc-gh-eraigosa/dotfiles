echo "crouton aliases installed, use crouton-help for more info."
alias startchroot='sudo enter-chroot -n precise'
alias startxfce4='sudo enter-chroot -n xfce startxfce4'
#alias startunity='sudo initctl stop powerd;sudo enter-chroot -n unity startunity'
# alias startunity='sudo enter-chroot -n unity startunity'
alias startunity='sudo enter-chroot -n trusty exec env XMETHOD=xorg startunity'
alias startunityw='sudo enter-chroot -n trusty  exec env XMETHOD=xiwi startunity'
alias startunitycmd='sudo enter-chroot -n trusty'

function crouton-help {
    cat <<HELP_TXT

  Summary:

    Helper for interacting with crouton chroot setup on ChromeOS.  Setup a 
    config in .bashrc for chroot name, otherwise we default to unity as
    the name of the chroot.  XMETHOD targets supported atm are xiwi and xorg.

    export CHROOT_NAME=trusty

  Available Commands:

    crouton-help : get the help text
    
    crouton-start [METHOD] : start the last XMETHOD we used, if method is set
                             then start one of these types of methods:
                             cmd  - command prompt only
                             xorg - full video , switch with ctl-alt-shift <arrows>
                             xiwi - use the chromeapp to connect and switch 
    crouton-run [command] : run a xiwi window command.  example: crouton-run terminator
    crouton-update : update the crouton image
    crouton-clean  : clean up the crouton scripts and alias
HELP_TXT

}

function crouton-start {
   _METHOD=$1
   [[ -z "$CHROOT_NAME" ]] && CHROOT_NAME=trusty
   if [ -z "$_METHOD" ] ; then
       if [ -f ~/.config/crouton_method.config ]; then
           _METHOD=$(cat ~/.config/crouton_method.config)
       else
           _METHOD=cmd
       fi
   fi
   case "$_METHOD" in
       cmd) 
           echo "start using cmd"
           echo $_METHOD> ~/.config/crouton_method.config
           sudo enter-chroot -n $CHROOT_NAME
           ;;
       xorg) 
           echo "start using xorg"
           echo $_METHOD> ~/.config/crouton_method.config
           sudo enter-chroot -n $CHROOT_NAME exec env XMETHOD=xorg startunity
           ;;
       xiwi) 
           echo "start using xiwi"
           echo $_METHOD> ~/.config/crouton_method.config
           sudo enter-chroot -n $CHROOT_NAME exec env XMETHOD=xiwi startunity
           ;;
       *)
           echo "ERROR start method not supported"
           crouton-help
           ;;
   esac
}
function crouton-run {
    COMMAND=$1
    if [ -z "$COMMAND" ]; then
       echo "crouton-run <command> : run a xiwi command in chrome window"
    else
       sudo enter-chroot -b xiwi $COMMAND -n $CHROOT_NAME
    fi
}
function crouton-update {
   [[ -z "$CHROOT_NAME" ]] && CHROOT_NAME=trusty
   cd ~/Downloads
   [ ! -f ./crouton ] && curl https://goo.gl/fd3zc > ./crouton
   [ ! -f ./crouton-alias.sh ] && curl https://raw.githubusercontent.com/wenlock/myhome/master/opt/bin/crouton-alias.sh > ./crouton-alias.sh
   sudo sh ./crouton -u -n $CHROOT_NAME
}
function crouton-clean {
    cd ~/Downloads
    [ -f ./crouton ] && rm -f ./crouton
    [ -f ./crouton-alias.sh ] && rm -f ./crouton-alias.sh
    echo "clean done"
}
