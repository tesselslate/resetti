#!/bin/bash
# Run this script from inside a clone of GLFW.

git checkout 3.3.8
curl -J -O https://raw.githubusercontent.com/woofdoggo/resetti/dev/contrib/glfw-xinput.patch
git apply glfw-xinput.patch
cmake -S . -B build -D BUILD_SHARED_LIBS=ON
cd build
make

mkdir -p deb/usr/lib/glfw
cp src/libglfw.so deb/usr/lib/glfw
mkdir -p deb/DEBIAN
{
    echo "Package: libglfw-mcsr"
    echo "Version: 3.3.8"
    echo "Architecture: amd64"
    echo "Maintainer: woofdoggo <woofwoofdoggo@protonmail.com>"
    echo "Description: Patched version of GLFW for minecraft speedruns"
} > deb/DEBIAN/control
dpkg-deb --build --root-owner-group deb glfw.deb
