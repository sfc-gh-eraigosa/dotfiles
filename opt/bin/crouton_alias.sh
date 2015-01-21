set -o vi
alias startchroot='sudo enter-chroot -n precise'
alias startxfce4='sudo enter-chroot -n xfce startxfce4'
#alias startunity='sudo initctl stop powerd;sudo enter-chroot -n unity startunity'
# alias startunity='sudo enter-chroot -n unity startunity'
alias startunity='sudo enter-chroot -n trusty startunity'
alias startunityw='sudo enter-chroot -n trusty  exec env XMETHOD=xiwi startunity'
alias startunityw='sudo enter-chroot -n trusty  exec env XMETHOD=xorg startunity'
