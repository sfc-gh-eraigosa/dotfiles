#!/bin/bash
# Find bad files in current folder and below.
COUNTER=0
TO_PROC=$(find . |wc -l)
echo "Will process ${TO_PROC} files"

find .  |
{
while read -r FILE
do
  if ! stat "$FILE" > /dev/null 2<&1 ; then
     echo "BAD File found: $FILE"
  else
    COUNTER=$((COUNTER + 1))
  fi
  echo -e -n "Processed files ... ${COUNTER}"\\r
#  if ! ((COUNTER % 1000)); then
#    echo -e -n "Processed files ... ${COUNTER}"\\r
#  else
#  fi
done
echo "Good files found = ${COUNTER}"
}
