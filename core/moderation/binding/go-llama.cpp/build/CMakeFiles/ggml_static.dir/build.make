# CMAKE generated file: DO NOT EDIT!
# Generated by "Unix Makefiles" Generator, CMake Version 3.28

# Delete rule output on recipe failure.
.DELETE_ON_ERROR:

#=============================================================================
# Special targets provided by cmake.

# Disable implicit rules so canonical targets will work.
.SUFFIXES:

# Disable VCS-based implicit rules.
% : %,v

# Disable VCS-based implicit rules.
% : RCS/%

# Disable VCS-based implicit rules.
% : RCS/%,v

# Disable VCS-based implicit rules.
% : SCCS/s.%

# Disable VCS-based implicit rules.
% : s.%

.SUFFIXES: .hpux_make_needs_suffix_list

# Command-line flag to silence nested $(MAKE).
$(VERBOSE)MAKESILENT = -s

#Suppress display of executed commands.
$(VERBOSE).SILENT:

# A target that is always out of date.
cmake_force:
.PHONY : cmake_force

#=============================================================================
# Set environment variables for the build.

# The shell in which to execute make rules.
SHELL = /bin/sh

# The CMake executable.
CMAKE_COMMAND = /usr/bin/cmake

# The command to remove a file.
RM = /usr/bin/cmake -E rm -f

# Escaping for special characters.
EQUALS = =

# The top-level source directory on which CMake was run.
CMAKE_SOURCE_DIR = /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp

# The top-level build directory on which CMake was run.
CMAKE_BINARY_DIR = /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build

# Include any dependencies generated for this target.
include CMakeFiles/ggml_static.dir/depend.make
# Include any dependencies generated by the compiler for this target.
include CMakeFiles/ggml_static.dir/compiler_depend.make

# Include the progress variables for this target.
include CMakeFiles/ggml_static.dir/progress.make

# Include the compile flags for this target's objects.
include CMakeFiles/ggml_static.dir/flags.make

# Object files for target ggml_static
ggml_static_OBJECTS =

# External object files for target ggml_static
ggml_static_EXTERNAL_OBJECTS = \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles/ggml.dir/ggml.c.o" \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles/ggml.dir/ggml-alloc.c.o" \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles/ggml.dir/k_quants.c.o"

libggml_static.a: CMakeFiles/ggml.dir/ggml.c.o
libggml_static.a: CMakeFiles/ggml.dir/ggml-alloc.c.o
libggml_static.a: CMakeFiles/ggml.dir/k_quants.c.o
libggml_static.a: CMakeFiles/ggml_static.dir/build.make
libggml_static.a: CMakeFiles/ggml_static.dir/link.txt
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --bold --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_1) "Linking C static library libggml_static.a"
	$(CMAKE_COMMAND) -P CMakeFiles/ggml_static.dir/cmake_clean_target.cmake
	$(CMAKE_COMMAND) -E cmake_link_script CMakeFiles/ggml_static.dir/link.txt --verbose=$(VERBOSE)

# Rule to build all files generated by this target.
CMakeFiles/ggml_static.dir/build: libggml_static.a
.PHONY : CMakeFiles/ggml_static.dir/build

CMakeFiles/ggml_static.dir/clean:
	$(CMAKE_COMMAND) -P CMakeFiles/ggml_static.dir/cmake_clean.cmake
.PHONY : CMakeFiles/ggml_static.dir/clean

CMakeFiles/ggml_static.dir/depend:
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build && $(CMAKE_COMMAND) -E cmake_depends "Unix Makefiles" /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles/ggml_static.dir/DependInfo.cmake "--color=$(COLOR)"
.PHONY : CMakeFiles/ggml_static.dir/depend

