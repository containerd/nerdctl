# To install CNI, see https://github.com/containerd/containerd/blob/release/1.7/script/setup/install-cni-windows

$ErrorActionPreference = "Stop"

#install containerd
$version=$env:ctrdVersion
echo "Installing containerd $version"
curl.exe -L https://github.com/containerd/containerd/releases/download/v$version/containerd-$version-windows-amd64.tar.gz -o containerd-windows-amd64.tar.gz
tar.exe xvf containerd-windows-amd64.tar.gz
mkdir -force "$Env:ProgramFiles\containerd"
cp ./bin/* "$Env:ProgramFiles\containerd"

& $Env:ProgramFiles\containerd\containerd.exe config default | Out-File "$Env:ProgramFiles\containerd\config.toml" -Encoding ascii
& $Env:ProgramFiles\containerd\containerd.exe --register-service
Start-Service containerd

echo "configuration complete! Printing configuration..."
echo "Service:"
get-service containerd
echo "cni configuration"
cat "$Env:ProgramFiles\containerd\cni\conf\0-containerd-nat.conf"
ls "$Env:ProgramFiles\containerd\cni\bin"
echo "containerd install"
ls "$Env:ProgramFiles\containerd\"
& "$Env:ProgramFiles\containerd\containerd.exe" --version
