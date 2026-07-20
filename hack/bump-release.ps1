param(
    [string]$Version
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Version) -or $Version -notmatch '^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') {
    throw "Version must look like v0.1.2 or v0.1.2-alpha.1. Got: $Version"
}

$repoRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$image = "ghcr.io/k8s-operators-devops/k8s-maintenance-operator"

$updates = @(
    @{
        Path = "README.md"
        Patterns = @(
            @{ From = 'k8s-maintenance-operator/v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?/deploy/install\.yaml'; To = "k8s-maintenance-operator/$Version/deploy/install.yaml" },
            @{ From = [regex]::Escape($image) + ':v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?'; To = "$image`:$Version" }
        )
    },
    @{
        Path = "deploy/install.yaml"
        Patterns = @(
            @{ From = [regex]::Escape($image) + ':v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?'; To = "$image`:$Version" }
        )
    },
    @{
        Path = ".github/workflows/publish-image.yml"
        Patterns = @(
            @{ From = 'for example v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?'; To = "for example $Version" },
            @{ From = 'default: "v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?"'; To = "default: `"$Version`"" }
        )
    },
    @{
        Path = "examples/gitops/argocd/application.yaml"
        Patterns = @(
            @{ From = 'targetRevision: v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?'; To = "targetRevision: $Version" }
        )
    },
    @{
        Path = "examples/gitops/flux/kustomization.yaml"
        Patterns = @(
            @{ From = 'tag: v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?'; To = "tag: $Version" }
        )
    },
    @{
        Path = ".github/ISSUE_TEMPLATE/bug_report.yml"
        Patterns = @(
            @{ From = 'operator v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?'; To = "operator $Version" }
        )
    }
)

foreach ($update in $updates) {
    $path = Join-Path $repoRoot $update.Path
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Missing expected file: $($update.Path)"
    }

    $content = Get-Content -LiteralPath $path -Raw
    foreach ($pattern in $update.Patterns) {
        $content = [regex]::Replace($content, $pattern.From, $pattern.To)
    }
    Set-Content -LiteralPath $path -Value $content -NoNewline
    Write-Host "Updated $($update.Path)"
}

Write-Host ""
Write-Host "Release references now point to $Version."
Write-Host "Review CHANGELOG.md manually and add the release notes before tagging."
