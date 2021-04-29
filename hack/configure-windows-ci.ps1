$ErrorActionPreference = "Stop"

#install golang
choco install --limitoutput --no-progress -y golang

#install containerd
$Version="1.5.7"
curl.exe -L https://github.com/containerd/containerd/releases/download/v$Version/containerd-$Version-windows-amd64.tar.gz -o containerd-windows-amd64.tar.gz
tar xvf containerd-windows-amd64.tar.gz
mkdir -force "$Env:ProgramFiles\containerd"
mv ./bin/* "$Env:ProgramFiles\containerd"

& $Env:ProgramFiles\containerd\containerd.exe config default | Out-File "$Env:ProgramFiles\containerd\config.toml" -Encoding ascii
& $Env:ProgramFiles\containerd\containerd.exe --register-service
Start-Service containerd

#configure cni
mkdir -force "$Env:ProgramFiles\containerd\cni\bin"
mkdir -force "$Env:ProgramFiles\containerd\cni\conf"
curl.exe -LO https://github.com/microsoft/windows-container-networking/releases/download/v0.2.0/windows-container-networking-cni-amd64-v0.2.0.zip
Expand-Archive windows-container-networking-cni-amd64-v0.2.0.zip -DestinationPath "$Env:ProgramFiles\containerd\cni\bin" -Force

curl.exe -LO https://raw.githubusercontent.com/microsoft/SDN/master/Kubernetes/windows/hns.psm1
ipmo ./hns.psm1

# cirrus already has nat net work configured for docker.  We can re-use that for testing
$sn=(get-hnsnetwork | ? Name -Like "nat" | select -ExpandProperty subnets)
$subnet=$sn.AddressPrefix
$gateway=$sn.GatewayAddress
@"
{
    "cniVersion": "0.2.0",
    "name": "nat",
    "type": "nat",
    "master": "Ethernet",
    "ipam": {
        "subnet": "$subnet",
        "routes": [
            {
                "gateway": "$gateway"
            }
        ]
    },
    "capabilities": {
        "portMappings": true,
        "dns": true
    }
}
"@ | Set-Content "$Env:ProgramFiles\containerd\cni\conf\0-containerd-nat.conf" -Force

echo "configuration complete! Printing configuration..."
echo "Service:"
get-service containerd
echo "cni configuraiton"
cat "$Env:ProgramFiles\containerd\cni\conf\0-containerd-nat.conf"
ls "$Env:ProgramFiles\containerd\cni\bin"
echo "containerd install"
ls "$Env:ProgramFiles\containerd\"
& "$Env:ProgramFiles\containerd\containerd.exe" --version
