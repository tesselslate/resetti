#!/bin/bash

git clone https://github.com/glfw/glfw
cd glfw
git checkout 3.3.8
curl -J -O https://raw.githubusercontent.com/tesselslate/resetti/dev/contrib/glfw-xinput.patch
git apply glfw-xinput.patch
cmake -S . -B build -D BUILD_SHARED_LIBS=ON
cd build
make
