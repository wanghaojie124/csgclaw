$ErrorActionPreference = "Stop"

$App = if ($env:APP) { $env:APP } else { "csgclaw" }
$Repo = if ($env:REPO) { $env:REPO } else { "OpenCSGs/csgclaw" }
$Version = if ($env:VERSION) { $env:VERSION } else { "latest" }
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $HOME "AppData\Local\Programs\csgclaw\bin" }
$BaseUrl = if ($env:BASE_URL) { $env:BASE_URL } else { "https://csgclaw.opencsg.com/releases" }

function Resolve-Arch {
    switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()) {
        "x64" { return "amd64" }
        "arm64" { return "arm64" }
        default { throw "Unsupported architecture: $([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture)" }
    }
}

function Resolve-Version {
    if ($Version -ne "latest") {
        return $Version
    }

    $apiUrl = "https://api.github.com/repos/$Repo/releases/latest"
    $release = Invoke-RestMethod -Uri $apiUrl
    if (-not $release.tag_name) {
        throw "Failed to resolve latest release from $apiUrl"
    }
    return $release.tag_name
}

function Test-PathContainsInstallDir {
    $pathEntries = ($env:Path -split ';') | ForEach-Object { $_.TrimEnd('\') }
    $target = $InstallDir.TrimEnd('\')
    return $pathEntries -contains $target
}

throw "Unsupported platform: Windows. Prebuilt csgclaw binaries currently support macOS arm64 and Linux amd64 only."
