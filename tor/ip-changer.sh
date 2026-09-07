#!/bin/bash

TORRC_FILE="/etc/tor/torrc"
SOCKS_PORT="9050"
LISTEN_ADDRESS="0.0.0.0"  # < - - - - - 

[[ "$UID" -ne 0 ]] && {
    echo "Script must be run as root."
    exit 1
}

install_packages() {
    local distro
    distro=$(awk -F= '/^NAME/{print $2}' /etc/os-release)
    distro=${distro//\"/}
    
    echo "Attempting to install curl and tor on $distro..."

    case "$distro" in
        *"Ubuntu"* | *"Debian"*)
            apt-get update
            apt-get install -y curl tor
            ;;
        *"Fedora"* | *"CentOS"* | *"Red Hat"* | *"Amazon Linux"*)
            yum update
            yum install -y curl tor
            ;;
        *"Arch"*)
            pacman -Syu --noconfirm curl tor
            ;;
        *)
            echo "Unsupported distribution: $distro. Please install curl and tor manually."
            exit 1
            ;;
    esac
}

configure_tor() {
    echo "Configuring Tor to listen on $LISTEN_ADDRESS:$SOCKS_PORT..."
    
    if grep -q "^SocksPort" "$TORRC_FILE"; then
        sed -i "/^SocksPort/c\SocksPort $LISTEN_ADDRESS:$SOCKS_PORT" "$TORRC_FILE"
    else
        echo "SocksPort $LISTEN_ADDRESS:$SOCKS_PORT" >> "$TORRC_FILE"
    fi

    if ! grep -q "^ExitNodes" "$TORRC_FILE"; then
        echo "Adding ExitNodes directive for better reliability."
        echo "ExitNodes {us}, {gb}, {fr}, {de}" >> "$TORRC_FILE"
    fi
}


if ! command -v curl &> /dev/null || ! command -v tor &> /dev/null; then
    echo "Installing curl and tor"
    install_packages
fi

configure_tor

if ! systemctl --quiet is-active tor.service; then
    echo "Starting tor service"
    systemctl restart tor.service 
else
    echo "Tor service is active. Reloading to apply configuration changes."
    systemctl reload tor.service 
fi

get_ip() {
    local url get_ip ip
    url="https://checkip.amazonaws.com"
    get_ip=$(curl -s -x socks5h://127.0.0.1:$SOCKS_PORT "$url")
    ip=$(echo "$get_ip" | grep -oP '\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}')
    echo "$ip"
}

change_ip() {
    echo "Reloading tor service"
    systemctl reload tor.service
    echo -e "\033[34mNew IP address: $(get_ip)\033[0m"
}

clear
cat << EOF

██╗██╗  ██╗██╗        ███████╗██╗      ██████╗ ██╗    ██╗███████╗██████╗ 
██║╚██╗██╔╝██║        ██╔════╝██║     ██╔═══██╗██║    ██║██╔════╝██╔══██╗
██║ ╚███╔╝ ██║        █████╗  ██║     ██║   ██║██║ █╗ ██║█████╗  ██████╔╝
██║ ██╔██╗ ██║        ██╔══╝  ██║     ██║   ██║██║███╗██║██╔══╝  ██╔══██╗
██║██╔╝ ██╗██║███████╗██║     ███████╗╚██████╔╝╚███╔███╔╝███████╗██║  ██║
╚═╝╚═╝  ╚═╝╚═╝╚══════╝╚═╝     ╚══════╝ ╚═════╝  ╚══╝╚══╝ ╚══════╝╚═╝  ╚═╝
                                                                         
                                                                                                       
EOF

echo -e "\n\033[32mInitial Tor IP address: $(get_ip)\033[0m\n"

while true; do
    read -rp $'\033[34mTime Interval ?  (type 0 for infinite ): \033[0m' interval
    read -rp $'\033[34mHow many IP ? (0 for infinite): \033[0m' times

    if [ "$interval" -eq "0" ] || [ "$times" -eq "0" ]; then
        echo "Starting infinite IP changes (interval range: 10-20s)"
        while true; do
            change_ip
            interval=$(shuf -i 10-20 -n 1) 
            sleep "$interval"
        done
    else
        for ((i=0; i< times; i++)); do
            change_ip
            sleep "$interval"
        done
        echo -e "\033[32m\nFinished cycling IP address $times times.\033[0m"
        exit 0
    fi
done
