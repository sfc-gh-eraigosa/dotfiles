#!/usr/bin/env bash

# check_and_sync_forks.sh
# Checks all forks for the authenticated user and syncs them if requested.

SYNC=false
FORCE=false
for arg in "$@"; do
  case $arg in
    --sync)
      SYNC=true
      shift
      ;;
    --force)
      FORCE=true
      shift
      ;;
  esac
done

echo "Fetching list of forks..."
FORKS=$(gh repo list --fork --limit 1000 --json nameWithOwner -q '.[].nameWithOwner')

if [[ -z "$FORKS" ]]; then
  echo "No forks found."
  exit 0
fi

TOTAL=$(echo "$FORKS" | wc -l | xargs)
echo "Found $TOTAL forks. Checking status..."

OUT_OF_DATE=0
SYNCED=0
FAILED=0

TMP_DIR=$(mktemp -d)

OUT_OF_DATE_REPOS=()

check_repo() {
  local REPO=$1
  local SYNC=$2
  local FORCE=$3
  
  local REPO_INFO=$(gh api "repos/$REPO" --jq '{parent: .parent.full_name, branch: .default_branch}' 2>/dev/null || echo "")
  local PARENT=$(echo "$REPO_INFO" | jq -r '.parent // empty')
  local BRANCH=$(echo "$REPO_INFO" | jq -r '.branch // empty')

  if [[ -n "$PARENT" && -n "$BRANCH" ]]; then
    local OWNER=${REPO%/*}
    local COMPARE=$(gh api "repos/$PARENT/compare/$BRANCH...$OWNER:$BRANCH" 2>/dev/null || echo "")
    local STATUS=$(echo "$COMPARE" | jq -r '.status // "unknown"')
    local BEHIND=$(echo "$COMPARE" | jq -r '.behind_by // 0')
    
    if [[ "$STATUS" == "behind" || "$STATUS" == "diverged" ]] && [[ "$BEHIND" -gt 0 ]]; then
      echo "[$REPO] Status: $STATUS, Behind by: $BEHIND commits."
      
      if [[ "$SYNC" == "true" ]]; then
        echo "  -> Syncing $REPO..."
        local SYNC_CMD="gh repo sync $REPO"
        if [[ "$STATUS" == "diverged" || "$FORCE" == "true" ]]; then
            SYNC_CMD="gh repo sync $REPO --force"
        fi
        
        if $SYNC_CMD >/dev/null 2>&1; then
           echo "  -> Successfully synced $REPO"
           echo "synced $REPO" > "$TMP_DIR/${REPO//\//_}.status"
        else
           echo "  -> Failed to sync $REPO. Might need 'workflow' scope."
           echo "failed $REPO" > "$TMP_DIR/${REPO//\//_}.status"
        fi
      else
        echo "out_of_date $REPO" > "$TMP_DIR/${REPO//\//_}.status"
      fi
    fi
  fi
}

export -f check_repo
export TMP_DIR

# Use parallel checking for speed (10 parallel jobs)
echo "$FORKS" | xargs -n 1 -P 10 -I {} bash -c "check_repo \"{}\" \"$SYNC\" \"$FORCE\""

for f in "$TMP_DIR"/*.status; do
  if [[ -f "$f" ]]; then
    read -r VAL REPO_NAME < "$f"
    if [[ "$VAL" == "out_of_date" ]]; then 
      ((OUT_OF_DATE++))
      OUT_OF_DATE_REPOS+=("$REPO_NAME")
    fi
    if [[ "$VAL" == "synced" ]]; then ((SYNCED++)); ((OUT_OF_DATE++)); fi
    if [[ "$VAL" == "failed" ]]; then 
      ((FAILED++))
      ((OUT_OF_DATE++))
      OUT_OF_DATE_REPOS+=("$REPO_NAME (Failed to sync)")
    fi
  fi
done

rm -rf "$TMP_DIR"

echo ""
echo "Summary:"
if [[ ${#OUT_OF_DATE_REPOS[@]} -gt 0 ]]; then
  echo "  Out of date forks:"
  for r in "${OUT_OF_DATE_REPOS[@]}"; do
    echo "    - $r"
  done
else
  echo "  Out of date forks: 0"
fi

if [[ "$SYNC" == "true" ]]; then
  echo "  Successfully synced: $SYNCED"
  echo "  Failed to sync: $FAILED"
  if [[ "$FAILED" -gt 0 ]]; then
    echo "  (Note: Failures often happen if upstream added workflows. Run: gh auth refresh -s workflow)"
  fi
fi
