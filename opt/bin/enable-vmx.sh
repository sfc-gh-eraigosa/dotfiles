#!/bin/sh -e 
C_ROOT=''
C_KERNEL=''
##
## Exits the script with exit code $1, spitting out message $@ to stderr
error() {
    local ecode="$1"
    shift
    echo "$*" 1>&2
    exit "$ecode"
}

##
## Ensure this script is run from a Cros shell (as chronos)
if [ $USER != 'chronos' ]; then
  error 1 "This script has to be run from a Cros shell (as chronos) - exiting!"
fi

#Following routine borrowed from @drinkcat ;)
ROOTDEVICE="`rootdev -d -s`"
if [ -z "$ROOTDEVICE" ]; then
    error 1 "Cannot find root device."
fi
if [ ! -b "$ROOTDEVICE" ]; then
    error 1 "$ROOTDEVICE is not a block device."
fi
# If $ROOTDEVICE ends with a number (e.g. mmcblk0), partitions are named
# ${ROOTDEVICE}pX (e.g. mmcblk0p1). If not (e.g. sda), they are named
# ${ROOTDEVICE}X (e.g. sda1).
ROOTDEVICEPREFIX="$ROOTDEVICE"
if [ "${ROOTDEVICE%[0-9]}" != "$ROOTDEVICE" ]; then
    ROOTDEVICEPREFIX="${ROOTDEVICE}p"
fi

##
## 0.Disable verified boot : (You could do this later, but you have to do this before step 10. It won't boot up otherwise.)
echo "## 0.Disable verified boot"
echo "Changing system to allow booting unsigned images"
sudo crossystem dev_boot_signed_only=0 || ( echo; error 2 "*** Couldn't disable 'verified boot'" )
sleep 3 && echo

##
## 1.Get current root & assign current kernel
echo "## 1.Get current root & assign current kernel"
echo -n "Determining current kernel = "
C_ROOT=`rootdev -s`
if [ $C_ROOT = ${ROOTDEVICEPREFIX}3 ]; then C_KERNEL=2; else C_KERNEL=4; fi
echo "$C_KERNEL"
sleep 3 && echo

##
## 2.Copy the existing kernel into a file
echo "## 2.Copy the existing kernel into a file"
echo "Changing to ~/Downloads directory for file operations"
cd $HOME/Downloads
echo "Copying current kernel into file = kernel$C_KERNEL"
sudo dd if=${ROOTDEVICEPREFIX}$C_KERNEL of=kernel$C_KERNEL || ( echo; error 2 "*** Couldn't create kernel file" )
sleep 3 && echo

##
## 3.Get the kernel configuration
echo "## 3.Get the kernel configuration"
echo -n "Grabbing tail-end of kernel in file = "
sudo vbutil_kernel --verify kernel$C_KERNEL --verbose | tail -1 > vmxoff-config$C_KERNEL.txt || ( echo; error 2 "*** Couldn't get config params" )
echo "vmxoff-config$C_KERNEL.txt"
sleep 3 && echo

##
## 4.Edit the config files and add 'disablevmx=off' to the config line
echo "## 4.Edit the config files and add 'disablevmx=off' to the config line"
if grep --color disablevmx=off vmxoff-config$C_KERNEL.txt ; then echo; error 1 "*** Kernel already modified - exiting"; fi 
echo -n "Editing vmxoff-config$C_KERNEL.txt to append = "
sed -i -e 's/$/ disablevmx=off lsm.module_locking=0/' vmxoff-config$C_KERNEL.txt || ( echo; error 2 "*** Couldn't edit config file" )
echo "disablevmx=off"
sleep 3 && echo

##
## 5.Repack the kernel
echo "## 5.Repack the kernel"
echo "Repacking the kernel from new file = repacked$C_KERNEL"
sudo vbutil_kernel --repack repacked$C_KERNEL \
--signprivate /usr/share/vboot/devkeys/kernel_data_key.vbprivk \
--oldblob kernel$C_KERNEL \
--keyblock /usr/share/vboot/devkeys/kernel.keyblock \
--config vmxoff-config$C_KERNEL.txt || ( echo; error 2 "*** Couldn't repack kernel" )
sleep 3 && echo

##
## 6.Verify the new kernel:
echo "## 6.Verify the new kernel:"
echo "Verifying file = repacked$C_KERNEL"
sudo vbutil_kernel --verify repacked$C_KERNEL --verbose || ( echo; error 2 "*** Couldn't verify new kernel" )
sleep 3 && echo

##
## 7.If verifying works, then it should just be a matter of:
echo "## 7.If verifying works, then it should just be a matter of:"
echo "Copying modified kernel back to = ${ROOTDEVICEPREFIX}$C_KERNEL"
echo -n "[ press ENTER to continue - Ctrl-C to abort ] "; read DOIT
sudo dd if=repacked$C_KERNEL of=${ROOTDEVICEPREFIX}$C_KERNEL || ( echo; error 2 "*** Couldn't create new kernel" )
sleep 3 && echo

##
## 8.reboot, and enjoy VT-x extensions
echo "## 8.reboot, and enjoy VT-x extensions"
echo "All done - rebooting..."
echo -n "[ press ENTER to continue - Ctrl-C to abort ] "; read DOIT
sudo reboot
exit