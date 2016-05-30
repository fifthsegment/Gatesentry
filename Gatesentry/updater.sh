#!/bin/bash
FILE="/tmp/gatesentry-update-available"
UPFILE="/etc/gatesentry/updates/update.zip"
UPFILESH="/etc/gatesentry/updates/update.sh"
UPDIR="/etc/gatesentry/updates/"
SIGFILE="container.zip.gpg"
CONTAINERFILE="container.zip"
CONTAINERDIRNAME="container"
CONTAINERDIR=$UPDIR$CONTAINERDIRNAME
fprint="5D9670853E58CF00B3ED39165B5009680E942A0E"
verify_signature() {
    local file=$1 out=
    if out=$(gpg --status-fd 1 --verify "$file" 2>/dev/null) &&
       echo "$out" | grep -qs "^\[GNUPG:\] VALIDSIG $fprint " &&
       echo "$out" | grep -qs "^\[GNUPG:\] TRUST_ULTIMATE\$"; then
        return 0
    else
        echo "$out" >&2
        return 1
    fi
}
if [ -f "$FILE" ];
then
   link=$(head -n 1 $FILE)
   echo $link
   sudo wget -O $UPFILE $link
   cd $UPDIR
   sudo unzip $UPFILE -d "package"
   cd package
   echo "BEGIN VERIFICATION"
   echo $UPDIR$SIGFILE
   sudo mv $SIGFILE ../
   cd ../
   sudo chown pi:pi $UPDIR
  if verify_signature $UPDIR$SIGFILE; then
      echo "OKAY"
      gpg --output container.zip --decrypt $SIGFILE
      echo "EXTRACTED $CONTAINERFILE"
      sudo unzip $CONTAINERFILE -d "container"
      sudo chown pi:pi container
      cd $CONTAINERDIR
      sudo chmod +x update.sh
	echo "CURRENT DIRECTORY"
	ls
      sudo ./update.sh
      # sudo chmod +x $UPFILESH
      # `$UPFILESH`
  #   sudo rm $FILE
  else
      echo "UNVERIFIED"
  fi
  sudo rm -rf $UPDIR
  sudo mkdir $UPDIR
#   sudo rm $FILE

  sudo rm $FILE
else
   echo "Reload File $FILE does not exist" >&2
fi

