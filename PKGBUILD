# Maintainer: rokiri
pkgname=clap-bin
pkgver=0.1.0
pkgrel=1
pkgdesc="A tiny Go-based package manager for GitHub apps, AUR, and snap"
arch=('x86_64')
url="https://github.com/ckklapp/clap"
license=('GPL3')
provides=('clap')
conflicts=('clap')
depends=('git' 'pacman' 'sudo')
options=('!strip' '!debug')
source=("clap-linux-amd64::https://github.com/ckklapp/clappm/releases/download/v${pkgver}/clap-linux-amd64")
sha256sums=('SKIP')

package() {
  install -Dm755 "${srcdir}/clap-linux-amd64" "${pkgdir}/usr/bin/clap"
}
