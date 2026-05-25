#!/bin/bash
# wifi-manage.sh: Manage WiFi BSSID locking and roaming

CONNECTION_NAME=$(nmcli -t -f NAME,TYPE,DEVICE connection show --active | grep -E "wifi|802-11-wireless" | cut -d: -f1 | head -n 1)

if [[ -z "$CONNECTION_NAME" ]]; then
    echo "Error: No active WiFi connection found."
    exit 1
fi

show_usage() {
    echo "Usage: wifi-manage [scan|lock|roam|status]"
    echo "  scan   - List available APs and signal strengths"
    echo "  lock   - Scan and prompt to lock to a specific BSSID"
    echo "  roam   - Reset to default roaming mode (clear BSSID lock)"
    echo "  status - Show current BSSID lock status"
}

case "$1" in
    status)
        CURRENT_BSSID=$(nmcli -f 802-11-wireless.bssid connection show "$CONNECTION_NAME" | awk '{print $2}')
        if [[ "$CURRENT_BSSID" == "--" || -z "$CURRENT_BSSID" ]]; then
            echo "Status: Roaming mode (no BSSID lock)"
        else
            echo "Status: Locked to BSSID $CURRENT_BSSID"
        fi
        ;;
    scan)
        echo "Scanning for Access Points for '$CONNECTION_NAME'..."
        nmcli -f BSSID,SIGNAL,BARS,CHAN,SSID dev wifi list | grep "$(nmcli -t -f active,ssid dev wifi | grep '^yes' | cut -d: -f2)"
        ;;
    roam)
        echo "Reverting to roaming mode for '$CONNECTION_NAME'..."
        nmcli connection modify "$CONNECTION_NAME" 802-11-wireless.bssid ""
        nmcli connection up "$CONNECTION_NAME"
        echo "Roaming enabled."
        ;;
    lock)
        echo "Available Access Points:"
        # Use line numbers for selection
        MAPFILE=()
        i=1
        SSID=$(nmcli -t -f active,ssid dev wifi | grep '^yes' | cut -d: -f2)
        while IFS= read -r line; do
            BSSID=$(echo "$line" | awk '{print $1}')
            MAPFILE+=("$BSSID")
            printf "[%d] %s\n" "$i" "$line"
            ((i++))
        done < <(nmcli -t -f BSSID,SIGNAL,BARS,CHAN,SSID dev wifi list | grep ":$SSID$")

        if [[ ${#MAPFILE[@]} -eq 0 ]]; then
            echo "No APs found for SSID '$SSID'."
            exit 1
        fi

        echo -n "Select an AP to lock into (1-${#MAPFILE[@]}): "
        read -r choice
        if [[ "$choice" -ge 1 && "$choice" -le "${#MAPFILE[@]}" ]]; then
            SELECTED_BSSID=${MAPFILE[$((choice-1))]}
            echo "Locking '$CONNECTION_NAME' to BSSID $SELECTED_BSSID..."
            nmcli connection modify "$CONNECTION_NAME" 802-11-wireless.bssid "$SELECTED_BSSID"
            nmcli connection up "$CONNECTION_NAME"
            echo "Lock applied."
        else
            echo "Invalid selection."
            exit 1
        fi
        ;;
    *)
        show_usage
        exit 1
        ;;
esac
