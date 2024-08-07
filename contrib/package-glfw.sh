#!/bin/bash
# Run this script from inside a clone of GLFW.

sh contrib/build-glfw.sh

mkdir -p deb/usr/lib/glfw
cp src/libglfw.so deb/usr/lib/glfw
mkdir -p deb/DEBIAN
{
    echo "Package: libglfw-mcsr"
    echo "Version: 3.3.8"
    echo "Architecture: amd64"
    echo "Maintainer: tesselslate <woofwoofdoggo@protonmail.com>"
    echo "Description: Patched version of GLFW for minecraft speedruns"
} > deb/DEBIAN/control
dpkg-deb --build --root-owner-group deb glfw.deb
