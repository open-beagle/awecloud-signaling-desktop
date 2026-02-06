# 安装自签名证书到系统信任列表
# 需要管理员权限运行

param(
    [Parameter(Mandatory=$true, HelpMessage="证书文件路径")]
    [string]$CertFile
)

# 检查是否以管理员权限运行
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
$isAdmin = $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    Write-Error "此脚本需要管理员权限运行"
    Write-Host "请右键点击 PowerShell，选择'以管理员身份运行'，然后重新执行此脚本"
    exit 1
}

# 检查证书文件是否存在
if (-not (Test-Path $CertFile)) {
    Write-Error "证书文件不存在: $CertFile"
    exit 1
}

Write-Host "正在安装证书: $CertFile" -ForegroundColor Green

try {
    # 导入证书到受信任的根证书颁发机构
    certutil -addstore -f "ROOT" $CertFile
    
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✓ 证书已成功安装到 Windows 系统信任列表" -ForegroundColor Green
        Write-Host "请重启 Desktop 应用以使更改生效" -ForegroundColor Yellow
    } else {
        Write-Error "证书安装失败，错误代码: $LASTEXITCODE"
        exit 1
    }
} catch {
    Write-Error "证书安装过程中发生错误: $_"
    exit 1
}
