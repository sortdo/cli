# Installs sortdo, Sort's command line, on Windows:
#
#   irm https://sort.do/install.ps1 | iex
#
# It downloads the release for this system from https://github.com/sortdo/cli/releases, checks its SHA-256 against
# the release's checksums.txt, puts sortdo.exe in %LOCALAPPDATA%\Programs\sortdo and adds that folder to your PATH.
#
#   $env:SORTDO_VERSION = '1.2.3'       Install this version instead of the latest.
#   $env:SORTDO_INSTALL_DIR = '<folder>' Install here instead.
#   $env:SORTDO_INSTALL_SKILL = '1'      Install the agent skill without asking ('0' skips it).
#   $env:SORTDO_RELEASES_URL = '<url>'   Download from this mirror of github.com/sortdo/cli instead (for tests and mirrors).

function Install-Sortdo {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $repo = if ($env:SORTDO_RELEASES_URL) { $env:SORTDO_RELEASES_URL } else { 'https://github.com/sortdo/cli' }
    $dir = if ($env:SORTDO_INSTALL_DIR) { $env:SORTDO_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\sortdo' }
    $arch = switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "sortdo install: $($env:PROCESSOR_ARCHITECTURE) processors are not supported." }
    }
    $asset = "sortdo_windows_$arch.zip"
    $base = if ($env:SORTDO_VERSION) { "$repo/releases/download/v$($env:SORTDO_VERSION.TrimStart('v'))" } else { "$repo/releases/latest/download" }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("sortdo-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading sortdo (windows $arch)..."
        Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')
        Invoke-WebRequest -UseBasicParsing -Uri "$base/$asset" -OutFile (Join-Path $tmp $asset)
        $expected = Get-Content (Join-Path $tmp 'checksums.txt') | ForEach-Object {
            $fields = $_ -split '\s+'
            if ($fields.Count -eq 2 -and $fields[1].TrimStart('*') -eq $asset) { $fields[0] }
        } | Select-Object -First 1
        if (-not $expected) { throw "sortdo install: checksums.txt has no entry for $asset." }
        $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $asset)).Hash
        if ($actual -ne $expected) { throw 'sortdo install: the download does not match its checksum. Nothing was installed.' }

        Expand-Archive -Path (Join-Path $tmp $asset) -DestinationPath $tmp -Force
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        $target = Join-Path $dir 'sortdo.exe'
        # A running sortdo.exe cannot be overwritten, but it can be moved aside; sortdo removes it on its next start.
        if (Test-Path $target) { Move-Item -Force $target "$target.old" }
        Move-Item -Force (Join-Path $tmp 'sortdo.exe') $target
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
    Write-Host "Installed $(& $target --version) in $target"

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not (($userPath -split ';') -contains $dir)) {
        [Environment]::SetEnvironmentVariable('Path', (($userPath, $dir) -join ';').TrimStart(';'), 'User')
        Write-Host "Added $dir to your PATH. Open a new terminal to use it."
    }
    if (-not (($env:Path -split ';') -contains $dir)) { $env:Path = "$env:Path;$dir" }

    $answer = $env:SORTDO_INSTALL_SKILL
    if (-not $answer -and -not [Console]::IsInputRedirected -and [Environment]::UserInteractive) {
        $reply = Read-Host 'Teach your coding agents (Claude Code, Codex, Cursor and others) to use sortdo? [Y/n]'
        $answer = if ($reply -match '^[nN]') { '0' } else { '1' }
    }
    if ($answer -eq '1') { & $target skill install } else { Write-Host 'For coding agents, run: sortdo skill install' }
    Write-Host ''
    Write-Host 'Next: sortdo login'
}

Install-Sortdo
