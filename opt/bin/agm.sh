# Script to source
#
#


function  AGM_check_agm
{
 MSG=""
 if [ -f ~/bin/agm/.origin.dat ]
 then
    FORIGIN="$(cat ~/bin/agm/.origin.dat)"
    if [ ! -f $FORIGIN ]
    then
       rm -f ~/bin/agm/.origin.dat
    else
       if [ "$(cat ~/bin/agm/agm.sh | md5sum)" != "$(cat $FORIGIN| md5sum)" ]
       then
          MSG="A different version ($FORIGIN) of your agm.sh copy is found. You may need to install it with: $FORIGIN setup ; source ~/bin/agm/agm.sh"
          return
       fi
    fi
 fi
 if [ "$AGM_MD5" != "$(cat ~/bin/agm/agm.sh | md5sum)" ]
 then
    MSG="please, reload agm with 'source ~/bin/agm/agm.sh'. "
 fi
}

function AGM_version
{
echo "0.3e"
}

function agm-setup
{
 if [ -f ~/bin/agm/prepare-commit-msg ]
 then
    AGM_VER_FOUND="$(awk -F'|' '$1 ~ /version$/ { printf "%s\n",$2 }' ~/bin/agm/prepare-commit-msg)"
    if [ "$(AGM_version)" != "$AGM_VER_FOUND" ]
    then
       rm -f ~/bin/agm/prepare-commit-msg
       echo "~/bin/agm/prepare-commit-msg out of date. Need to be updated."
    fi
 fi
 if [ ! -f ~/bin/agm/prepare-commit-msg ]
 then
    mkdir -p ~/bin/agm
    cat > ~/bin/agm/prepare-commit-msg << __END 
#!/bin/bash
#
# AGM hook to report current AGM defect or UserStory to the commit message.

 if [ "\$2" = "hook" ] && [ "\$3" = "version" ]
 then
    echo "# AGM hook version|$(AGM_version)|" > "\$1"
    exit 
 fi

 if [ -r ~/bin/agm/agm.sh ]
 then
    export AGM_ID=""
    # Loading agm.sh script. It loads automatically the correct AGM_ID, thanks to the name of the current branch.
    source ~/bin/agm/agm.sh
    echo "Selected AGM: \$AGM_ID"

    if [ "\$AGM_ID" != "" ]
    then
       if [ "\$(echo "\$AGM_ID" | grep '^Innovation')" != "" ]
       then
          if [ "\$(grep -ie "Innovation:" "\$1")" = "" ]
          then
             echo "Creating message..."
             cp -p "\$1" "\$1".bak
             printf "Innovation: \$AGM_Subject\n\$(cat "\$1".bak)" > "\$1"
             rm -f "\$1".bak
          fi
       else
          if [ "\$(grep -ie "\\(defect\\|userstory\\)#" "\$1")" != "" ]
          then
             echo "Updating message..."
             sed --in-place 's/\(defect\|userstory\)\#[0-9 ]*:/'"\$AGM_ID"' :/gi' "\$1"
          else
             echo "Creating message..."
             cp -p "\$1" "\$1".bak
             printf "\$AGM_ID : \$(cat "\$1".bak)" > "\$1"
             rm -f "\$1".bak
          fi
       fi
    fi
 fi

# vim: syntax=sh
__END
   chmod +x ~/bin/agm/prepare-commit-msg
   echo "installed central ~/bin/agm/prepare-commit-msg"
 fi

 if [ "$AGM_CUR_REPO_PATH" != "" ]
 then
    if [ -r "$AGM_CUR_REPO_PATH/.git/hooks/prepare-commit-msg" ]
    then
       if [ "$(grep "AGM hook version" "$AGM_CUR_REPO_PATH/.git/hooks/prepare-commit-msg")" != "" ]
       then
          rm -f $AGM_CUR_REPO_PATH/.git/hooks/prepare-commit-msg
          ln -sf ~/bin/agm/prepare-commit-msg $AGM_CUR_REPO_PATH/.git/hooks
          echo "AGM git hook re-installed."
       else
          echo "Oops... there is another prepare-commit-msg hook in place. You have to edit it, and add this line in front of any code:
~/bin/agm/prepare-commit-msg \"\$1\" \"\$2\" \"\$3\" "
       fi
    else
       ln -sf ~/bin/agm/prepare-commit-msg $AGM_CUR_REPO_PATH/.git/hooks
       echo "AGM git hook installed."
    fi
 else
    echo "You are not in a git repository. Nothing more to do there."   
 fi
 
}

function agm-configure
{
 if [ "$(echo $PROMPT_COMMAND | grep AGM_show)" != "" ] && [ "$1" != "--force" ]
 then
    echo "AGM tools is already configured, as PROMPT_COMMAND is set with AGM_show call. If you need to edit it, update your bashrc or .bash_profile."
    return 1
 fi
 typeset GIT=True
 __git_ps1 2>/dev/null >/dev/null
 if [ $? -ne 0 ]
 then
    echo "For your information: You do not have GIT prompt loaded... You will see a limited number of prompt.

On Fedora, you should install git (yum install git) and load /usr/share/git-core/contrib/completion/git-prompt.sh in your ~ /.bashrc
On Ubuntu, you should just install git (sudo apt-get install git)"
    typeset EXCLUDE="-and ! -path '*git*/*.prpt'"
 else
    typeset EXCLUDE=""
 fi
 echo "agm-configure is going to configure your prompt.
To simplify your prompt, the following list are provided by the script. You will be able to update it later from your .bashrc:

First, review the list and then select the right one.
Press Enter to continue.
"
 read
 eval "find ~/bin/agm/ -name \*.prpt $EXCLUDE" | while read PRPT_FILE
 do
    FILE="$(echo $PRPT_FILE | sed 's|^.*/bin/agm/\(.*\)\.prpt|\1|g')"
    echo "
---------------------------------------------------------------------------------------
$PRPT_FILE
---------------------------------------------------------------------------------------"
    cat "$PRPT_FILE" | grep -v -i -e type:
 done | more
 echo "---------------------------------------------------------------------------------------"

 select PRPT in $(eval "find ~/bin/agm/ -name \*.prpt $EXCLUDE") exit
 do
    if [ "$PRPT" != "exit" ] 
    then
       # Re-initializing AGM_PRPT options
       unset AGM_PRPT_ESC_PS1_COLOR AGM_PRPT_FOR_PS1 AGM_PRPT_PS1
       eval "$(cat $PRPT | grep -i -e "type: *" |grep -v -e "type: *#" | sed 's/^.*type: *//gi')"
    fi
    break
 done
 if [ "$PRPT" = "exit" ] 
 then
    return
 fi
 echo "The prompt has been set on your default shell.
Do you want to install it in your .bashrc?"
 while true
 do
    read -n 1 -p "y or N (Default)? " ANS
    case $ANS in
      y | Y )
         ANS=Yes
         break;;
      n | N | "")
         ANS=No
         break;;
    esac
    printf "\nPlease type y or n.\n"
 done
 if [ $ANS = Yes ]
 then
    if [ ! -f ~/.bashrc ] || [ "$(cat ~/.bashrc | grep -e "^ *(source|\.) (~/)*\.agmrc")" = "" ]
    then
       echo "source ~/.agmrc" >> ~/.bashrc
    fi
    cat $PRPT | grep -i -e type: | sed 's/^.*type: *//gi' > ~/.agmrc
    printf "\n.bashrc updated. All other sessions will need to reload the .bashrc to get the new prompt setting.\n"
 else
    printf "\nTo add the prompt at login, add the following lines to your login script (.profile, .bash_profile, .bashrc, ...):\n----------\n"
    cat $PRPT | grep -i -e type: | sed 's/^.*type: *//gi'
    echo "----------"
 fi
 printf "\nAGM Configuration done.\n"
}

function agm-help
{
 if [ "$1" = "" ]
 then
    echo "
agm-help [<AgmCommand>]: This help. Recall it with AGM function name to have more details on this AGM function.
agm-create [<options>] : Register a new AGM ID.
agm [<options>]        : Interactive helper to register a new AGM ID.
agm-attach <Number>    : To attach an AGM ID to the current repository/directory.
agm-detach             : To detach the attached AGM ID of the current repository/directory.
agm-done [<Number>]    : To close the current or another opened AGM ID.
agm-list [<Number>]    : To get list of active AGM ID or get detail on an AGM ID.
agm-cd <options>       : To move to another tracked branched repository.
cd :<options>          : alias to agm-cd features.
agm-pushd <options>    : like cd, but uses 'bash' pushd/popd.
pushd :<options>       : alias to agm-pushd features.
agm-configure [--force]: To configure your prompt."
    return
 fi

 case "$1" in
    agm-setup)
      echo "Use this function to configure your local repository with the agm hook for git."
      ;;
    agm-configure)
      typeset GIT=True
      __git_ps1 2>/dev/null >/dev/null
      if [ $? -ne 0 ]
      then
         typeset GIT=False
      fi
      echo "To gain on agm, I strongly suggest you to update your dynamic prompt (PROMPT_COMMAND) and add it to your ~/.bashrc.
Some prompt has been pre-defined. Using '[1magm-configure[0m', you will be able to choose the one you want and activate it.

You can contribute and share your prompt. simply create a .prpt.

The content of that file is shown as is, without prefixed 'type: ' string.
Lines prefixed by 'type: ' are going to be saved to the user's .bashrc file.

For details, see : https://lacws.emea.hpqcorp.net/tikiwiki/tiki-index.php?page=GIT+Configuration
"
       ;;
    agm-help)
       echo "$1 [<AgmCommand>]
where 
- [<AgmCommand>] : Optional. Provide the agm function help."
       ;;
    agm-detach)
       echo "$1 
This will simply detach your current repository/branch or simple directory to the current AGM ID."
       ;;
    agm-attach)
       echo "$1 ['All'] <Number>
where:
- ['All']  : This string inform that, all repository current branch are tracked by this AGM ID.
- <Number> : Save the AGM Number (AGM ID) to become the active one for this branched repository"
       ;;
    agm-done)
       echo "$1 [<AGM ID>]
where:
- [<AGM ID>] : Optional. Is a string representing 1 AGM ID. Regular expression is used. So, you can write $1 1375 if this string is uniq.

By default, this function will close the current AGM ID on all linked repositories/branches/directories."
       ;;
    agm-list)
       echo "$1 [<Number>]
This function provides the list of AGM ID currently active/open.
where 
- <Number> : Optional. When an AGM number is provided, agm-list will provide details on this tracked AGM ID."
       ;;
    popd)
       echo "$1
This is an agm internal alias. When agm is configured, popd with refresh your AGM variable, from the new path."
       ;;
    agm-cd|cd|pushd)
       echo "agm-cd|agm-popd <'?'|'??'|'root'|'$'|'next'|'+'|'prev'|'previous'|'-'|'1'..'9'|RegEx> ['.'|RelativePath]
OR
cd|pushd [1m:[0m<'?'|'??'|'root'|'$'|'next'|'+'|'prev'|'previous'|'-'|'1'..'9'|RegEx> ['.'|RelativePath]
where
- '?'                   : Provides the list of repository index available for the current AGM ID.
- '??'                  : Provides the list of all AGM ID attached to repositories.
- 'root'|'$'            : It will change to the repository root directory.
- 'next'|'+'            : It will search for the next repo in the list of tracked repo and change to its root directory.
- 'prev'|'previous'|'-' : It will search for the previous repo in the list of tracked repo and change to its root directory.
- '1'..'9'              : It will search the indexed repo in the list of tracked repo and change to its root directory.
- RegEx                 : Regular expression to find the tracked repo. Only the first found is used to change to its root directory.
- '.'                   : Optional. if you are staying on the same repository, Request to stay in the current directory.
- RelativePath          : Optional. agm-cd will change to the subdirectories of the selected repository."
       ;;
     agm-create)
       echo "$1 [--attach] [--attach-all] <D|U|I> <AGM Number> <AGM Summary>
where:
--attach     : To attach the new created AGM to the current repository.
--attach-all : To attach the new created AGM to ALL repositories. I do not recommend it, but it is possible to do it. :-)
<D|U|I>      : This is the type of AGM ID to create. D stands for Defect, U for UserStory and I for Innovative.
               An innovative artifact is not a work tracked by the AGM solution. 
               Use it if there is no AGM UserStory or Defect for the change you are doing. The Innovative number can be any number >=1
<AGM Number> : This is the number of this AGM ID to create. 
<Summary>    : This is the short AGM description. Defect/UserStory list is kept in ~/.agm_db file. For a new defect/UserStory, this summary is required.

NOTE: git commit won't add any Innovative#Number to the commit message. But the Summary prefixed by 'Innovation: ' is added to the commit message.

NOTE: There is currently no links to the AGM service. So, you need to maintain the AGM service manually yourself."

       ;;
     agm)
       echo "$1 [--attach] [--attach-all] 
where:
--attach     : To attach the new created AGM to the current repository.
--attach-all : To attach the new created AGM to ALL repositories. I do not recommend it, but it is possible to do it. :-)

This is an interactive creation of a Agile Manager artifact (Defect or UserStory). You just need to answer to questions.

NOTE: There is currently no links to the AGM service. So, you need to maintain the AGM service manually yourself.

If you want to create an agm directly without interaction, uses agm-create. Type agm-help agm-create for more information."

       ;;
  esac

}

function AGM_check_update
{
 CUR_REPO_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null)"
 if [ "$CUR_REPO_BRANCH" != "$AGM_CUR_REPO_BRANCH" ]
 then
    AGM_refresh
 fi
}

function AGM_refresh
{
 # This function loads some important AGM variables used for prompt and some functions.
 # List of Variables to set:
 # - AGM_CUR_REPO_PATH  : Repository PATH. This means, AGM_CUR_PATH to be empty.
 # - AGM_CUR_REPO_NAME  : Repository Name. basename of AGM_CUR_REPO_PATH.  This means, AGM_CUR_PATH to be empty.
 # - AGM_CUR_REPO_BRANCH: Repository Branch Name. This means, AGM_CUR_PATH to be empty.
 # - AGM_CUR_REPO       : Repository index from the list of Repository Name/branch/name. 
 # - AGM_ID             : AGM ID
 # - AGM_Subject        : AGM Summary
 # - AGM_CUR_PATH       : AGM ID registered PATH. This means, AGM_CUR_REPO_PATH, AGM_CUR_REPO_NAME and AGM_CUR_REPO_BRANCH to be empty.

 # Re-initializing published variables to re-update. AGM_ID and AGM_Subject are kept, refreshed only if ID changed.
 export AGM_CUR_REPO_PATH="$(git rev-parse --show-toplevel 2>/dev/null)"
 AGM_CUR_REPO_NAME=""
 AGM_CUR_REPO_BRANCH=""
 AGM_CUR_REPO=""
 AGM_CUR_PATH=""
 if [ "$AGM_CUR_REPO_PATH" != "" ]
 then
     export AGM_CUR_REPO_NAME="$(basename "$AGM_CUR_REPO_PATH")"
     export AGM_CUR_REPO_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
 fi
 AGM_REPO=~/.agm.repo
 if [ -f $AGM_REPO ]
 then
    if [ "$AGM_CUR_REPO_NAME" = "" ]
    then # Search for AGM_CUR_PATH and AGM_ID from agm.repo data file
       AGM_PATH_SEARCH="$(awk -F'|' '$1 ~ /^$/ && $3 ~ /^$/ { printf "%s\n",$4 }' $AGM_REPO )"
       for CHECK_PATH in $AGM_PATH_SEARCH
       do
         if [ "$(pwd | grep "$CHECK_PATH")" != "" ]
         then
            break
         fi
       done
       if [ "$(pwd | grep "$CHECK_PATH")" != "" ]
       then
          export AGM_CUR_PATH="$CHECK_PATH"
          CHECK_PATH="$(echo "$CHECK_PATH" | sed 's|/|\\/|g')"
          AGM_ID_SEARCH="$(awk -F'|' '$1 ~ /^$/ && $3 ~ /^$/ && $4 ~/^'"$CHECK_PATH"'/ { printf "%s\n",$2 }' $AGM_REPO )"
       fi
       unset CHECK_PATH AGM_PATH_SEARCH
    else # Search for AGM_ID from agm.repo data file
       AGM_ID_SEARCH="$(awk -F'|' '$1 ~ /^'"$AGM_CUR_REPO_NAME"'$/ && $3 ~ /^'"$AGM_CUR_REPO_BRANCH"'$/ { printf "%s\n",$2 }' $AGM_REPO  | head -n 1)"
       if [ "$AGM_ID_SEARCH" = "" ]
       then
          AGM_ID_SEARCH="$(awk -F'|' '$1 ~ /^A$/ { printf "%s\n",$2 }' $AGM_REPO | head -n 1)"
       fi
    fi
    if [ "$AGM_ID_SEARCH" != "" ]
    then
       if [ "$AGM_ID_SEARCH" != "$AGM_ID" ]
       then
          AGM_load $AGM_ID_SEARCH
       fi
       if [ "$AGM_ID" != "" ] 
       then
          typeset -i REPO_MAX
          REPO_MAX="$(awk -F'|' '$2 ~ /^'"$AGM_ID"'$/ { printf "%s\n",$1 }' $AGM_REPO | wc -l )"
          if [ $REPO_MAX -gt 0 ]
          then
             export AGM_CUR_REPO=":$(awk -F'|' '$2 ~ /^'"$AGM_ID"'$/ { printf "%s/%s|%s\n",$4,$1,$3 }' $AGM_REPO | awk -F'|' '$1 ~ /^'"$(echo $AGM_CUR_REPO_PATH | sed 's|/|\\/|g')"'$/ && $2 ~ /'"$AGM_CUR_REPO_BRANCH"'/  { printf "%i\n",FNR }' ) "
          fi
       fi
    else
       export AGM_ID=""
       export AGM_Subject=""
    fi
 fi
 unset AGM_ID_SEARCH AGM_REPO REPO_MAX AGM_PATH_SEARCH
}

function AGM_check_repo
{
 printf "$MSG"
 if [ "$AGM_CUR_REPO_NAME" != "" ]
 then
    if [ ! -r "$AGM_CUR_REPO_PATH/.git/hooks/prepare-commit-msg.ignore" ]
    then
       if [ ! -r "$AGM_CUR_REPO_PATH/.git/hooks/prepare-commit-msg" ]
       then
          echo "GIT hook for AGM is not installed. To get power on call agm-setup to install it."
       else
          touch /tmp/$$-prepare-commit-msg
          $AGM_CUR_REPO_PATH/.git/hooks/prepare-commit-msg "/tmp/$$-prepare-commit-msg" "hook" "version"
          AGM_VER_FOUND="$(awk -F'|' '$1 ~ /version$/ { printf "%s\n",$2 }' /tmp/$$-prepare-commit-msg)"
          if [ "$AGM_VER_FOUND" = "" ]
          then
             echo "GIT hook for AGM is not installed. To get power on call agm-setup to install it."
          else
             if [ "$(AGM_version)" != "$AGM_VER_FOUND" ]
             then
                echo "GIT hook for AGM is not updated. Please call agm-setup to update it." 
             fi
          fi
       fi
    fi
 fi
}

function AGM_set_MSG_file
{
 if [ "$AGM_CUR_REPO_PATH" != "" ]
 then
    AGM_MSG_FILE=$AGM_CUR_REPO_PATH/.git/agm.msg
 else
    if [ "$AGM_CUR_PATH" != "" ]
    then
       AGM_MSG_FILE=$AGM_CUR_PATH/.agm.msg
    else
       AGM_MSG_FILE=~/.agm.msg
    fi
 fi
}

function AGM_show
{
 # With 3 parameters provided, AGM_show will set the PS1. Usually, this is the call to use for setting PROMPT_COMMAND.
 # It is composed as follow: $1=PrefixPrompt, $2=Prefix AGM prompt, $3=Suffix Prompt.
 # With 2 parameters, AGM_Show will print out to the default output. AGM_show will behave like having 2 parameters if having less or more parameters to the call.
 # It is composed as follow: $1=Prefix AGM prompt, $2=Suffix AGM Prompt.
 AGM_check_agm
 AGM_set_MSG_file
 MSG="$(AGM_check_repo "$MSG")"
 if [ "$MSG" != "" ]
 then
    INFO="call agm-msg"
 else
    INFO=""
 fi
 echo "$MSG" > "$AGM_MSG_FILE"

 case $# in
    3)
      FORMAT="%i%a%s"
      AGM_PRPT_INFO='\e[1;31m!!! %s !!!\e[0m\n'
      AGM_PRPT_AGMID="$1$2"'\e[31m%s : '
      AGM_PRPT_AGMSUB='%s\e[0m'"$3" 
      AGM_PRPT_PS1=1
      ;;
    0)
      # The new format is based on 5 AGM_PRPT env Variables.
      # FORMAT defined the complete format of the prompt. %a, %s %i and %i will be replace by field format
      # %a is testing AGM_ID value. The AGM_PRPT_AGMID printf format will be set. The %s will be replaced by the value.
      # %I is testing INFO value. The AGM_PRPT_INFO printf format will be set. The %s will be replaced by the value.
      # %s is testing AGM_Subject value. The AGM_PRPT_AGMSUB printf format will be set. The %s will be replaced by the value.
      # %s is testing AGM_stats() value. The AGM_PRPT_STAT printf format will be set. The %s will be replaced by the value.
      # if AGM_PRPT_PS1 = 1, the result of this data wil be set in PS1 variable. every \e[[0-9;]m string will be encapsulated by \[ and \] for PS1 support.
      if [ "$AGM_PRPT_FORMAT" = "" ]
      then 
         FORMAT="%i%a%s"
      else
         FORMAT="$AGM_PRPT_FORMAT"
      fi
      ;;
    *)
      FORMAT="%i%a%s"
      AGM_PRPT_INFO='\e[1;31m!!! %s !!!\e[0m\n'
      AGM_PRPT_AGMID="$1"'\e[31m%s : '
      AGM_PRPT_AGMSUB='%s\e[0m'"$2" 
      ;;
 esac
 

 if [ "$AGM_ID" != "" ]      ; then dA="$(printf "$(echo "$AGM_PRPT_AGMID" | sed 's/\\/\\\\/g')" "$AGM_ID")"     ; fi
 if [ "$INFO" != "" ]        ; then dI="$(printf "$(echo "$AGM_PRPT_INFO"  | sed 's/\\/\\\\/g')" "$INFO")"       ; fi
 if [ "$AGM_Subject" != "" ] ; then dS="$(printf "$(echo "$AGM_PRPT_AGMSUB"| sed 's/\\/\\\\/g')" "$AGM_Subject")"; fi
 if [ "$AGM_PRPT_STAT" != "" ]
 then
    STAT="$(AGM_stats %s)"
    unset dSt # I do not know why thjs variable is set. unset it.
    if [ "$STAT" != "" ]     ; then dSt="$(printf "$(echo "$AGM_PRPT_STAT" | sed 's/\\/\\\\/g')" "$STAT")"       ; fi
 fi
 PRINT_FORMAT="$(echo "$FORMAT" |cut -d'|' -f1 | sed 's/%i/$dI/g
                                                      s/%a/$dA/g
                                                      s/%S/$dSt/g
                                                      s/%s/$dS/g')"
 # Get field values to print out.
 eval "PRINT_FORMAT=\"$PRINT_FORMAT\""

 if [ "$AGM_PRPT_ESC_PS1_COLOR" = 1 ] 
 then # Propose to add \[ and \] for recognized Color ansi code. This helps the prompt management (cursor position at prompt)
      PRINT_FORMAT="$(echo "$PRINT_FORMAT" | sed 's/\(\\e\[[0-9;]*m\)/\\[\1\\]/g')"
 fi

 if [ "$AGM_PRPT_FOR_PS1" = 1 ]
 then
     # AGM_PRPT_FOR_PS1 must be set to 1 if escape caracters have to be kept. This is required for PS1 setting, like those done by __git_ps1
     # ex: __git_ps1 "$(AGM_show)" "%s"
     # or if you write something like PS1="$(AGM_show)"
     PRINT_FORMAT="$(echo "$PRINT_FORMAT" | sed 's/\\/\\\\/g')"
 fi

 if [ "$AGM_PRPT_PS1" = 1 ] 
 then
      eval "PS1=\"$PRINT_FORMAT\""
 else
      printf "$PRINT_FORMAT"
 fi
 
 unset MSG AGM_MSG_FILE FORMAT dA dI dS dSt PRINT_FORMAT
}

function agm-msg
{
 AGM_set_MSG_file
 if [ -r "$AGM_MSG_FILE" ]
 then
    cat "$AGM_MSG_FILE"
    rm -f "$AGM_MSG_FILE"
 fi
 unset AGM_MSG_FILE
}

function AGM_load
{ # Function to load some basic information on an AGM (ID + Subject). Regular expression on the AGM ID is possible.
 AGM_DB=~/.agm_db
 if [ ! -f $AGM_DB ]
 then
    AGM_ID=$1
    AGM_Subject=""
    return
 fi
 if [ "$1" = "" ]
 then
    AGM_Subject=""
 else
    AGM_ID_Found="$(awk -F'|' '$1 ~ /'$1'/ { printf "%s\n",$1 }' $AGM_DB)"
    if [ "$(echo "$AGM_ID_Found" | wc -w)" -gt 1 ]
    then
       echo "Warning! You have selected several AGM ID . You need to refine your request to select one only.
Selected AGM ID:
$AGM_ID_Found"
       export AGM_ID=""
       AGM_Subject=""
       unset AGM_ID_Found
    else
       AGM_ID="$AGM_ID_Found"
       unset AGM_ID_Found
    fi
    AGM_Subject="$(awk -F'|' '$1 ~ /'$1'/ { printf "%s\n",$2 }' $AGM_DB)"
 fi
 if [ "$AGM_Subject" = "" ]
 then
    AGM_ID=""
 fi
 export AGM_ID
 unset AGM_DB
}

function agm-list
{
 AGM_DB=~/.agm_db
 if [ "$1" = "" ]
 then
    awk -F'|' '{ printf("%s\t%s\n",$1,$2) }' $AGM_DB
    unset AGM_DB
    return
 fi
 echo "Getting information:"
 awk -F'|' '$1 ~ /'"$1"'/ { printf ("%s\t - %s\n",$1,$2) }' $AGM_DB
 printf "\nLinked to:\n"
 AGM_REPO=~/.agm.repo
 awk -F'|' '$2 ~ /'"$1"'/ { if ($3 != "")
                               {
                                printf ("Repo %-20s B:%-20s (%s)\n",$1,$3,$4)
                               }
                            else
                               {
                                printf ("Dir  %s\n",$4)
                               }
                          }'  $AGM_REPO | awk '{ printf ":%d %s\n",FNR,$0}'
 unset AGM_DB AGM_REPO
}

function agm_alias_popd
{
 \popd 
 AGM_refresh
}

function agm_alias_pushd
{
 if [ "$(echo "$1" | cut -c1)" = : ]
 then
    agm-cd pushd "$(echo $1 | sed 's/^://g')" "$2"
 else
    if [ "$1" = "" ]
    then
       \pushd 
    else
       \pushd "$1"
    fi
    AGM_refresh
 fi
}

function agm_alias_cd
{
 if [ "$(echo "$1" | cut -c1)" = : ]
 then
    agm-cd "$(echo $1 | sed 's/^://g')" "$2"
 else
    if [ "$1" = "" ]
    then
       \cd
    else
       \cd "$1"
    fi
    AGM_refresh
 fi
}

function agm-cd
{
 CD=cd
 if [ "$1" = pushd ]
 then
    CD=pushd
    shift
 fi

 AGM_refresh
 typeset -i REPO_MAX
 AGM_REPO=~/.agm.repo
 
 case "$1" in
   '')
     if [ "$AGM_CUR_REPO_PATH" != "" ]
     then
        echo "Opened AGM artifacts:"
        AGM_DB=~/.agm_db
        awk -F'|' '{ 
                     Cmd=sprintf("awk -F\"|\" \"\\$1 ~ /^%s$/ { print(\\$2) }\" %s",$2,"'"$AGM_DB"'")
                     Cmd | getline TITLE
                     close(Cmd)
                     if ($1 == "" ) 
                        { 
                         printf "%-20s %s - [1m%s[0m\n",$2,$4,TITLE
                        }
                     else
                        { 
                         printf "%-20s %s (%s-%s) - [1m%s[0m\n",$2,$4,$1,$3,TITLE
                        }
                   }' $AGM_REPO  | sort
        REPO_MAX="$(awk -F'|' '$2 ~ /^'"$AGM_ID"'$/ { printf "%s\n",$1 }' $AGM_REPO | wc -l )"
        if [ $REPO_MAX -gt 1 ] 
        then
           printf "\nList of attached repository/directories to '$AGM_ID':\n"
           awk -F'|' '$2 ~ /^'"$AGM_ID"'$/ { if ($3 != "")
                                      {
                                       printf "%-20s: %s/%s (%s)\n",$2,$4,$1,$3
                                      }
                                    else
                                      {
                                       printf "%-20s: %s \n",$2,$4
                                      }
                                  }' $AGM_REPO | awk '{ printf ":%d %s\n",FNR,$0} '
        else
           printf "\nNo more attached repository/directories to '$AGM_ID'.\n"
        fi
        return
     fi
     ;;
   '??' | '')
     awk -F'|' '{ if ($1 == "" ) 
                     { 
                      printf "%-20s %s\n",$2,$4
                     }
                  else
                     { 
                      printf "%-20s %s (%s-%s)\n",$2,$4,$1,$3
                     }
                }' $AGM_REPO 
     return
     ;;
   '?')
     awk -F'|' '$2 ~ /^'"$AGM_ID"'$/ { if ($3 != "")
                                {
                                 printf "%-20s: %s/%s (%s)\n",$2,$4,$1,$3
                                }
                              else
                                {
                                 printf "%-20s: %s \n",$2,$4
                                }
                            }' $AGM_REPO | awk '{ printf ":%d %s\n",FNR,$0} '
     return
     ;;
   root | $)
     NEW_PATH="$AGM_CUR_REPO_PATH"
     shift
     ;;
   next | + )
     REPO_MAX="$(awk -F'|' '$2 ~ /^'"$AGM_ID"'$/ { printf "%s\n",$1 }' $AGM_REPO | wc -l )"
     if [ $REPO_MAX -gt 0 ] 
     then
         if [ $AGM_CUR_REPO -eq $REPO_MAX ]
         then
            AGM_CUR_REPO=1
         else
            let AGM_CUR_REPO++
         fi
     fi
     NEW_PATH="$(awk   -F'|' '$2 ~ /'"$AGM_ID"'/ { printf "%s/%s\n",$4,$1 }' $AGM_REPO | awk '{ if ( FNR == '$AGM_CUR_REPO') printf "%s\n",$0} ')"
     NEW_BRANCH="$(awk -F'|' '$2 ~ /'"$AGM_ID"'/ { printf "%s\n",$3 }'       $AGM_REPO | awk '{ if ( FNR == '$AGM_CUR_REPO') printf "%s\n",$0} ')"
     shift
     ;;
   prev | previous | -)
     REPO_MAX="$(awk -F'|' '$2 ~ /^'"$AGM_ID"'$/ { printf "%s\n",$1 }' $AGM_REPO | wc -l )"
     if [ $REPO_MAX -gt 0 ] 
     then
         if [ $AGM_CUR_REPO -eq 1 ] && [ $REPO_MAX -gt 1 ]
         then
            AGM_CUR_REPO=$REPO_MAX
         else
            let AGM_CUR_REPO--
         fi
     fi
     NEW_PATH="$(awk   -F'|' '$2 ~ /'"$AGM_ID"'/ { printf "%s/%s\n",$4,$1 }' $AGM_REPO | awk '{ if ( FNR == '$AGM_CUR_REPO') printf "%s\n",$0} ')"
     NEW_BRANCH="$(awk -F'|' '$2 ~ /'"$AGM_ID"'/ { printf "%s\n",$3 }'       $AGM_REPO | awk '{ if ( FNR == '$AGM_CUR_REPO') printf "%s\n",$0} ')"
     shift
     ;;
   [1-9])
     REPO_MAX="$(awk -F'|' '$2 ~ /^'"$AGM_ID"'$/ { printf "%s\n",$1 }' $AGM_REPO | wc -l )"
     if [ $REPO_MAX -lt $1 ]
     then
        echo "AGM Repo Index $1 is not defined"
        unset REPO_MAX AGM_REPO
        return
     fi
     NEW_PATH="$(awk   -F'|' '$2 ~ /'"$AGM_ID"'/ { printf "%s/%s\n",$4,$1 }' $AGM_REPO | awk '{ if ( FNR == '$1') printf "%s\n",$0} ')"
     NEW_BRANCH="$(awk -F'|' '$2 ~ /'"$AGM_ID"'/ { printf "%s\n",$3 }'       $AGM_REPO | awk '{ if ( FNR == '$1') printf "%s\n",$0} ')"
     shift
     ;;
   *)
     if [ "$(awk   -F'|' '$0 ~ /'$1'/ { printf "%s/%s\n",$4,$1}' $AGM_REPO | wc -l)" -gt 1 ]
     then
        echo "Warning! You may need to refine your query, as several path has been found:
$(awk   -F'|' '$0 ~ /'$1'/ { if ($3 != "")
                                {
                                 printf "%-20s: %s/%s (%s)\n",$2,$4,$1,$3
                                }
                              else
                                {
                                 printf "%-20s: %s \n",$2,$4
                                }
                            }' $AGM_REPO)

Selecting the first one."
     fi
     NEW_PATH="$(awk   -F'|' '$0 ~ /'$1'/ { printf "%s/%s\n",$4,$1}' $AGM_REPO | head -n1)"
     NEW_BRANCH="$(awk -F'|' '$0 ~ /'$1'/ { printf "%s\n",$3 }'      $AGM_REPO | head -n1)"
     shift
     ;;
 esac
 if [ -d "$NEW_PATH" ]
 then
    PWD_SAVED="$(pwd)"
    if [ "$1" = "." ] && [ "$NEW_PATH" = "$AGM_CUR_REPO_PATH" ]
    then
       NEW_PATH="."
       CUR_PATH=True
    fi
    if [ "$1" != "" ] && [ "$1" != "." ]
    then
       if [ -d "$NEW_PATH/$1" ]
       then
          NEW_PATH="$NEW_PATH/$1"
       fi
    fi
    if [ "$NEW_PATH" != "." ] && [ "$PWD_SAVED" != "$NEW_PATH" ]
    then
       $CD "$NEW_PATH"
       if [ $? -ne 0 ]
       then
          return $?
       fi
    else
       NEW_PATH="$PWD_SAVED (no change)"
    fi
    if [ "$NEW_BRANCH" != "" ]
    then
       CUR_REPO_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null)"
       if [ "$NEW_BRANCH" != "$CUR_REPO_BRANCH" ]
       then
          git checkout $NEW_BRANCH
          if [ $? -ne 0 ]
          then
             echo "now: ${NEW_PATH}. But failed to activate the branch '$NEW_BRANCH'."
          else
             if [ "$CUR_PATH" = "" ]
             then
                echo "now: branch $NEW_BRANCH"
             else
                echo "now: $NEW_PATH, branch $NEW_BRANCH"
             fi
          fi
       else
          echo "now: $NEW_PATH"
       fi
    else
       echo "now: $NEW_PATH"
    fi
    unset PWD_SAVED
 else
    echo "Unable to change to '$NEW_PATH'"
 fi
 AGM_refresh
 unset REPO_MAX AGM_REPO CUR_REPO_BRANCH CUR_PATH NEW_PATH NEW_BRANCH
}

function agm-done
{
 if [ "$AGM_ID" = "" ] && [ "$1" = "" ]
 then
    echo "Nothing to do. If you want to close another agm, just call agm-done <AGM-ID>."
    return
 fi
 if [ "$1" != "" ]
 then
    AGM_load "$1"
 fi
 if [ "$AGM_ID" = "" ]
 then
    if [ "$1" != "" ]
    then
       echo "Unable to identify an opened AGM ID with '$1'."
       agm-list
    else
       echo "Nothing to do. If you want to close another agm, just call agm-done <AGM-ID>."
    fi
    return
 fi
 AGM_REPO_FILE=~/.agm.repo
 AGM_DB=~/.agm_db
 sed --in-place=.bak '/|'$AGM_ID'|/d' $AGM_REPO_FILE 
 sed --in-place=.bak '/'$AGM_ID'/d' $AGM_DB 
 unset AGM_REPO_FILE AGM_DB
 if [ "$(echo "$AGM_ID" | grep '^Innovation')" = "" ]
 then
    echo "$AGM_ID is closed. Remember to report it to AGM website."
 else
    echo "$AGM_ID is closed."
 fi
 export AGM_ID=""
 AGM_refresh
}

function agm-detach
{
 if [ "$1" = All ]
 then
    CUR_AGM_REPO="A"
    shift
 fi
 if [ "$1" != "" ]
 then
    AGM_load "$1"
 fi
 AGM_REPO_FILE=~/.agm.repo
 if [ "$AGM_ID" != "" ]
 then
    if [ "$CUR_AGM_REPO" = A ]
    then
       sed --in-place=.bak '/^'$AGM_CUR_REPO_NAME'|.*|'$AGM_CUR_REPO_BRANCH'/d' $AGM_REPO_FILE 
       echo "'$AGM_ID' detached for All repositories branches"
    else
       if [ "$AGM_CUR_REPO_PATH" != "" ]
       then
           CUR_AGM_REPO_PATH="$(dirname "$AGM_CUR_REPO_PATH" 2>/dev/null)"
           sed --in-place=.bak '/^'"$AGM_CUR_REPO_NAME"'|'"$AGM_ID"'|'"$AGM_CUR_REPO_BRANCH"'|'"$(echo "$CUR_AGM_REPO_PATH" | sed 's/\//\\\//g')"'/d' $AGM_REPO_FILE 
           echo "'$AGM_ID' detached for '$AGM_CUR_REPO_NAME' repository branch '$AGM_CUR_REPO_BRANCH'"
       else
           CUR_AGM_REPO_PATH="$(pwd)"
           sed --in-place=.bak '/^|'"$AGM_ID"'||'"$(echo "$CUR_AGM_REPO_PATH" | sed 's/\//\\\//g')"'/d' $AGM_REPO_FILE 
           echo "'$AGM_ID' detached for '$CUR_AGM_REPO_PATH' path"
       fi
    fi
 fi   
 AGM_refresh
 unset AGM_REPO_FILE CUR_AGM_REPO_PATH 
}

function AGM_stats
{ # Report stats on current opened AGMs. No colors.
 AGM_REPO_FILE=~/.agm.repo
 AGM_DB=~/.agm_db
 if [ "$1" = "" ]
 then
    FORMAT="(%s)"
 else
    FORMAT="$1"
 fi
 RESULT="$(awk -F "|" '$1 ~ /UserStory#/ { UserStory++ }
             $1 ~ /^Defect#/ { Defect++ }
             $1 ~ /^Innovation#/ { Innovative++ }
             BEGIN { UserStory=0; Defect=0; Innovative=0; }
             END {
                  sSep=""
                  if (UserStory>0)
                     {
                      sOut=sprintf("U:%d",UserStory);
                      sSep=" "
                     }
                  if (Defect>0)
                     {
                      sOut=sprintf("%s%sD:%d",sOut,sSep,Defect);
                      sSep=" "
                     }
                  if (Innovative>0)
                     {
                      sOut=sprintf("%s%sI:%d",sOut,sSep,Innovative);
                     }
                  if (sOut != "")
                     {
                      printf("%s",sOut)
                     }
                 }' $AGM_DB)"
 if [ "$RESULT" != "" ]
 then
    printf "$FORMAT" "$RESULT" 
 fi
 unset AGM_DB AGM_REPO_FILE FORMAT RESULT
}

function agm-attach
{
 if [ "$1" = All ]
 then
    CUR_AGM_REPO="A"
    shift
 fi
 if [ "$1" != "" ]
 then
    AGM_load $1
 fi
 AGM_REPO_FILE=~/.agm.repo
 if [ "$AGM_ID" != "" ]
 then
    if [ "$CUR_AGM_REPO" = A ]
    then
       sed --in-place=.bak '/^'$AGM_CUR_REPO_NAME'|.*|'$AGM_CUR_REPO_BRANCH'/d' $AGM_REPO_FILE 
       echo "'$AGM_ID' attached to All repository"
       echo "A|$AGM_ID|$AGM_CUR_REPO_BRANCH|$CUR_AGM_REPO_PATH" >> $AGM_REPO_FILE
    else
       if [ "$AGM_CUR_REPO_PATH" != "" ]
       then
           CUR_AGM_REPO_PATH="$(dirname "$AGM_CUR_REPO_PATH" 2>/dev/null)"
           sed --in-place=.bak '/^'"$AGM_CUR_REPO_NAME"'|.*|'"$AGM_CUR_REPO_BRANCH"'|'"$(echo "$CUR_AGM_REPO_PATH" | sed 's/\//\\\//g')"'/d' $AGM_REPO_FILE 
           echo "'$AGM_ID' attached to '$AGM_CUR_REPO_NAME' repository branch '$AGM_CUR_REPO_BRANCH'"
       else
           CUR_AGM_REPO_PATH="$(pwd)"
           sed --in-place=.bak '/^|.*||'"$(echo "$CUR_AGM_REPO_PATH" | sed 's/\//\\\//g')"'/d' $AGM_REPO_FILE 
           echo "'$AGM_ID' attached to '$CUR_AGM_REPO_PATH' path"
       fi
       echo "$AGM_CUR_REPO_NAME|$AGM_ID|$AGM_CUR_REPO_BRANCH|$CUR_AGM_REPO_PATH" >> $AGM_REPO_FILE
    fi
 else
    echo "No valid AGM defect or UserStory loaded. 
List of opened AGM artifacts:"
    agm-list
 fi   
 AGM_refresh
 unset AGM_REPO_FILE CUR_AGM_REPO_PATH 
}

function agm
{ # This function is an helper to create an AGM.
 typeset AGM_REPO=""
 while [ "$(echo "p$1" | cut -c1-3)" = "p--" ]
 do
    case "$1" in
     '--attach')
      AGM_REPO=T
      shift;;
     '--attach-all')
      AGM_REPO=A
      shift;;
    esac
 done
 if [ $# -ne 0 ]
 then
    unset AGM_TYPE AGM_REPO
    echo "agm is an AGM creation help. "
    agm-help agm
    return 1
 fi

 while [ "$AGM_TYPE" != "D" ] && [ "$AGM_TYPE" != "U" ]
 do
   read -p "Type D (for defect) or U (for UserStory) or I (for Innovation) or empty to exit: " -i U -n 1 AGM_TYPE
   if [ "$AGM_TYPE" = "" ]
   then
      unset AGM_TYPE AGM_REPO
      AGM_refresh
      return
   fi
   if [ "$AGM_TYPE" != "D" ] && [ "$AGM_TYPE" != "U" ] && [ "$AGM_TYPE" != "I" ]
   then
      printf "\nOnly 'D','I' or 'I' are valid.\n"
   fi
 done
 if [ "$AGM_TYPE" = "D" ]
 then
    AGM_TYPES="Defect"
 else
    if [ "$AGM_TYPE" = "U" ]
    then
       AGM_TYPES="UserStory"
    else
       AGM_TYPES="Innovation"
    fi
 fi
 typeset -i AGM_NUM
 printf "\nProvide the $AGM_TYPES number: (empty to exit)\n"
 read -p "$AGM_TYPES#" AGM_NUM
 if [ $AGM_NUM -eq 0 ]
 then
      unset AGM_TYPE AGM_TYPES AGM_NUM AGM_REPO
      AGM_refresh
    return
 fi
 AGM_load "$AGM_TYPES#$AGM_NUM"
 if [ "$AGM_ID" = "" ]
 then
    AGM_ID="$AGM_TYPES#$AGM_NUM"
 fi
 if [ "$AGM_TYPES" = Innovative ]
 then
       printf "Provide your innovative summary: (Not tracked by AGM web site)"
 else
    printf "Provide a summary of $AGM_ID:"
 fi
 read -i "$AGM_Subject" -p "=> " AGM_Subject
 if [ "$AGM_REPO" != "" ]
 then
    printf "You can automatically attach this new AGM to all repository (not recommended) or to your current directory or repository, or attach it later.\nSo, now, what do you want to do?
Type 'A' to attach '$AGM_ID' to ALL GIT repositories. Not recommended.
Type 'T' to attach to your current repository '$AGM_CUR_REPO_NAME' branch '$AGM_CUR_REPO_BRANCH'
Just press enter if you will attach it later.\n"
    AGM_REPO=""
    while [ "$AGM_REPO" != "A" ] && [ "$AGM_REPO" != "T" ] && [ "$AGM_REPO" != "NONE" ]
    do
      read -p "Type A or T or press enter:" -i T -n 1 AGM_REPO
      case "$AGM_REPO" in
        "" )
         echo "Not attached."
         AGM_REPO="NONE"
         AGM_ATTACH=""
         ;;
        "A" )
         AGM_ATTACH="--attach-all"
         ;;
        "T" )
         AGM_ATTACH="--attach"
         ;;
      esac
    done
 fi

 agm-create $AGM_ATTACH $AGM_TYPE $AGM_NUM "$AGM_Subject"

 unset AGM_ATTACH AGM_TYPE AGM_NUM AGM_Subject AGM_TYPES AGM_REPO
 AGM_refresh
}

function agm-create
{
 typeset AGM_REPO=""
 while [ "$(echo "p$1" | cut -c1-3)" = "p--" ]
 do
    case "$1" in
     '--attach')
      AGM_REPO=T
      shift;;
     '--attach-all')
      AGM_REPO=A
      shift;;
    esac
 done

 typeset AGM_TYPE
 if [ $# -ne 3 ]
 then
    echo "Missing parameters."
    agm-help agm-create
    return 1
 fi

 if [ "$1" != "D" ] && [ "$1" != "U" ] && [ "$1" != "I" ]
 then
    echo "Only 'D' or 'U' or 'I' are valid."
    agm-help agm-create
    return 1
 fi
 AGM_TYPEN=$1
 if [ "$AGM_TYPEN" = "D" ]
 then
    AGM_TYPEN="Defect"
 else
    if [ "$AGM_TYPEN" = "U" ]
    then
       AGM_TYPEN="UserStory"
    else
       AGM_TYPEN="Innovation"
    fi
 fi

 if [ "$2" -le 0 ]
 then
    echo "$AGM_TYPEN number invalid. Must be >0"
    return 1
 fi
 AGM_TYPE=$AGM_TYPEN
 export AGM_ID="$AGM_TYPE#$2"

 AGM_Subject="$3"

 AGM_DB=~/.agm_db
 AGM_Subject_old="$(awk -F'|' '$1 ~ /^'$AGM_ID'$/ { printf "%s\n",$2 }' $AGM_DB)"
 if [ "$AGM_ID" != "" ] && [ "$AGM_Subject" != "$AGM_Subject_old" ]
 then
    echo "Saving summary for ${AGM_ID}."
    sed --in-place=.bak '/'$AGM_ID'/d' $AGM_DB 
    echo "$AGM_ID|$AGM_Subject" >> $AGM_DB
 fi
 
 unset AGM_TYPEN AGM_TYPE AGM_Subject_old CUR_AGM_REPO AGM_REPO_FILE AGM_DB
 printf "AGM: $AGM_ID='$AGM_Subject' created.\n"

 case "$AGM_REPO" in
    "A")
       agm-attach All $AGM_ID
       ;;
    "T")   
       agm-attach $AGM_ID
       ;;
 esac
 unset AGM_REPO
 AGM_refresh
}

function AGM_copy_files
{          
 if [ "$SCRIPT" = "" ]
 then
    return
 fi
 install -m 755 $SCRIPT ~/bin/agm/
 cd $(dirname $SCRIPT)
 install -m 644 -t ~/bin/agm/ *.prpt
 if [ "$1" = "only-prpt" ] || [ "$1" = "" ]
 then
    ls */*.prpt | sed 's|/.*\.prpt||g' | sort -u | while read DEST
    do
       install -d -m 755 ~/bin/agm/$DEST 
       install -m 644 -t ~/bin/agm/$DEST $DEST/*.prpt
    done
 fi
}

if [ "$(basename -- $0)" != "agm.sh" ]
then
   export AGM_MD5=$(cat ~/bin/agm/agm.sh | md5sum)
   alias cd=agm_alias_cd
   alias pushd=agm_alias_pushd
   alias popd=agm_alias_popd

   AGM_refresh
else
   if [ $# -eq 0 ] 
   then
      echo "To install/update AGM function for bash, call '$0 setup'. Exiting."
      exit
   fi
   case "$1" in
      setup)
        SCRIPT=$0
        if [ ! -d ~/bin/agm ]
        then
           echo "Installing bash agm functions"
           mkdir -p ~/bin/agm
           echo "#AGM function
source ~/bin/agm/agm.sh" >> ~/.bashrc
           AGM_copy_files
           echo "agm.sh has been installed. your .bash_profile has been updated to load agm.sh, at each login. To load agm in your current bash shell, type '[1msource ~/bin/agm/agm.sh[0m'"
        else
           if [ "$(cat $0 | md5sum)" != "$(cat ~/bin/agm/agm.sh | md5sum)" ]
           then
              echo "Updating bash agm functions"
              AGM_copy_files
              echo "agm.sh has been updated. Refresh your bash environment, with '[1msource ~/bin/agm/agm.sh[0m'"
           else
              AGM_copy_files only-prpt
              echo "agm functions already up to date."
           fi
        fi
        AGM_REPO=~/.agm.repo
        if [ ! -f $AGM_REPO ]
        then
           touch $AGM_REPO
        fi
        AGM_DB=~/.agm_db
        if [ ! -f $AGM_DB ]
        then
           touch $AGM_DB
        fi
        unset AGM_REPO AGM_DB

        AGM_refresh
        agm-setup
        echo "$(cd $(dirname "$SCRIPT") ; pwd)/$(basename "$SCRIPT")" > ~/bin/agm/.origin.dat
        agm-help agm-configure
        ;;
      version)
        AGM_version
        ;;
      *)
        echo "To install/update AGM function for bash, call '$0 setup'. Exiting."
        ;;
   esac
fi
