# Install script for directory: /home/vadim/go/src/github.com/warpnet/go-llama.cpp/llama.cpp/examples

# Set the install prefix
if(NOT DEFINED CMAKE_INSTALL_PREFIX)
  set(CMAKE_INSTALL_PREFIX "/usr/local")
endif()
string(REGEX REPLACE "/$" "" CMAKE_INSTALL_PREFIX "${CMAKE_INSTALL_PREFIX}")

# Set the install configuration name.
if(NOT DEFINED CMAKE_INSTALL_CONFIG_NAME)
  if(BUILD_TYPE)
    string(REGEX REPLACE "^[^A-Za-z0-9_]+" ""
           CMAKE_INSTALL_CONFIG_NAME "${BUILD_TYPE}")
  else()
    set(CMAKE_INSTALL_CONFIG_NAME "Release")
  endif()
  message(STATUS "Install configuration: \"${CMAKE_INSTALL_CONFIG_NAME}\"")
endif()

# Set the component getting installed.
if(NOT CMAKE_INSTALL_COMPONENT)
  if(COMPONENT)
    message(STATUS "Install component: \"${COMPONENT}\"")
    set(CMAKE_INSTALL_COMPONENT "${COMPONENT}")
  else()
    set(CMAKE_INSTALL_COMPONENT)
  endif()
endif()

# Install shared libraries without execute permission?
if(NOT DEFINED CMAKE_INSTALL_SO_NO_EXE)
  set(CMAKE_INSTALL_SO_NO_EXE "1")
endif()

# Is this installation the result of a crosscompile?
if(NOT DEFINED CMAKE_CROSSCOMPILING)
  set(CMAKE_CROSSCOMPILING "FALSE")
endif()

# Set default install directory permissions.
if(NOT DEFINED CMAKE_OBJDUMP)
  set(CMAKE_OBJDUMP "/usr/bin/objdump")
endif()

if(NOT CMAKE_INSTALL_LOCAL_ONLY)
  # Include the install script for each subdirectory.
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/main/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/quantize/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/quantize-stats/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/perplexity/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/embedding/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/save-load-state/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/benchmark/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/baby-llama/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/train-text-from-scratch/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/convert-llama2c-to-ggml/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/simple/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/speculative/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/embd-input/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/llama-bench/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/beam-search/cmake_install.cmake")
  include("/home/vadim/go/src/github.com/warpnet/go-llama.cpp/build/examples/server/cmake_install.cmake")

endif()

