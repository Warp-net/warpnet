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
include common/CMakeFiles/common.dir/depend.make
# Include any dependencies generated by the compiler for this target.
include common/CMakeFiles/common.dir/compiler_depend.make

# Include the progress variables for this target.
include common/CMakeFiles/common.dir/progress.make

# Include the compile flags for this target's objects.
include common/CMakeFiles/common.dir/flags.make

common/CMakeFiles/common.dir/common.cpp.o: common/CMakeFiles/common.dir/flags.make
common/CMakeFiles/common.dir/common.cpp.o: /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/common.cpp
common/CMakeFiles/common.dir/common.cpp.o: common/CMakeFiles/common.dir/compiler_depend.ts
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_1) "Building CXX object common/CMakeFiles/common.dir/common.cpp.o"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -MD -MT common/CMakeFiles/common.dir/common.cpp.o -MF CMakeFiles/common.dir/common.cpp.o.d -o CMakeFiles/common.dir/common.cpp.o -c /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/common.cpp

common/CMakeFiles/common.dir/common.cpp.i: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Preprocessing CXX source to CMakeFiles/common.dir/common.cpp.i"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -E /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/common.cpp > CMakeFiles/common.dir/common.cpp.i

common/CMakeFiles/common.dir/common.cpp.s: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Compiling CXX source to assembly CMakeFiles/common.dir/common.cpp.s"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -S /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/common.cpp -o CMakeFiles/common.dir/common.cpp.s

common/CMakeFiles/common.dir/console.cpp.o: common/CMakeFiles/common.dir/flags.make
common/CMakeFiles/common.dir/console.cpp.o: /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/console.cpp
common/CMakeFiles/common.dir/console.cpp.o: common/CMakeFiles/common.dir/compiler_depend.ts
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_2) "Building CXX object common/CMakeFiles/common.dir/console.cpp.o"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -MD -MT common/CMakeFiles/common.dir/console.cpp.o -MF CMakeFiles/common.dir/console.cpp.o.d -o CMakeFiles/common.dir/console.cpp.o -c /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/console.cpp

common/CMakeFiles/common.dir/console.cpp.i: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Preprocessing CXX source to CMakeFiles/common.dir/console.cpp.i"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -E /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/console.cpp > CMakeFiles/common.dir/console.cpp.i

common/CMakeFiles/common.dir/console.cpp.s: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Compiling CXX source to assembly CMakeFiles/common.dir/console.cpp.s"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -S /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/console.cpp -o CMakeFiles/common.dir/console.cpp.s

common/CMakeFiles/common.dir/grammar-parser.cpp.o: common/CMakeFiles/common.dir/flags.make
common/CMakeFiles/common.dir/grammar-parser.cpp.o: /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/grammar-parser.cpp
common/CMakeFiles/common.dir/grammar-parser.cpp.o: common/CMakeFiles/common.dir/compiler_depend.ts
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_3) "Building CXX object common/CMakeFiles/common.dir/grammar-parser.cpp.o"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -MD -MT common/CMakeFiles/common.dir/grammar-parser.cpp.o -MF CMakeFiles/common.dir/grammar-parser.cpp.o.d -o CMakeFiles/common.dir/grammar-parser.cpp.o -c /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/grammar-parser.cpp

common/CMakeFiles/common.dir/grammar-parser.cpp.i: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Preprocessing CXX source to CMakeFiles/common.dir/grammar-parser.cpp.i"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -E /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/grammar-parser.cpp > CMakeFiles/common.dir/grammar-parser.cpp.i

common/CMakeFiles/common.dir/grammar-parser.cpp.s: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Compiling CXX source to assembly CMakeFiles/common.dir/grammar-parser.cpp.s"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -S /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common/grammar-parser.cpp -o CMakeFiles/common.dir/grammar-parser.cpp.s

common: common/CMakeFiles/common.dir/common.cpp.o
common: common/CMakeFiles/common.dir/console.cpp.o
common: common/CMakeFiles/common.dir/grammar-parser.cpp.o
common: common/CMakeFiles/common.dir/build.make
.PHONY : common

# Rule to build all files generated by this target.
common/CMakeFiles/common.dir/build: common
.PHONY : common/CMakeFiles/common.dir/build

common/CMakeFiles/common.dir/clean:
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common && $(CMAKE_COMMAND) -P CMakeFiles/common.dir/cmake_clean.cmake
.PHONY : common/CMakeFiles/common.dir/clean

common/CMakeFiles/common.dir/depend:
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build && $(CMAKE_COMMAND) -E cmake_depends "Unix Makefiles" /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/common /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common/CMakeFiles/common.dir/DependInfo.cmake "--color=$(COLOR)"
.PHONY : common/CMakeFiles/common.dir/depend

