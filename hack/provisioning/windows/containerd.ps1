$ErrorActionPreference = "Stop"

#install containerd
$version=$env:ctrdVersion
$expectedSha=$env:ctrdSha
echo "Installing containerd $version"
curl.exe -L https://github.com/containerd/containerd/releases/download/v$version/containerd-$version-windows-amd64.tar.gz -o containerd-windows-amd64.tar.gz

if ($expectedSha -eq "canary is volatile and I accept the risk") {
    echo "Skipping SHA256 verification (canary)"
} else {
    $expected = $expectedSha.ToLower()
    $actual = (Get-FileHash -Algorithm SHA256 containerd-windows-amd64.tar.gz).Hash.ToLower()
    if ($actual -ne $expected) {
        Write-Error "SHA256 mismatch for containerd-windows-amd64.tar.gz: expected $expected, got $actual"
        exit 1
    }
    echo "SHA256 verified: $actual"
}

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
cat "$Env:ProgramFiles\containerd\cni\conf\0-containerd-nat.conflist"
ls "$Env:ProgramFiles\containerd\cni\bin"
echo "containerd install"
ls "$Env:ProgramFiles\containerd\"
& "$Env:ProgramFiles\containerd\containerd.exe" --version
