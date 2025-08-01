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
include examples/baby-llama/CMakeFiles/baby-llama.dir/depend.make
# Include any dependencies generated by the compiler for this target.
include examples/baby-llama/CMakeFiles/baby-llama.dir/compiler_depend.make

# Include the progress variables for this target.
include examples/baby-llama/CMakeFiles/baby-llama.dir/progress.make

# Include the compile flags for this target's objects.
include examples/baby-llama/CMakeFiles/baby-llama.dir/flags.make

examples/baby-llama/CMakeFiles/baby-llama.dir/baby-llama.cpp.o: examples/baby-llama/CMakeFiles/baby-llama.dir/flags.make
examples/baby-llama/CMakeFiles/baby-llama.dir/baby-llama.cpp.o: /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/examples/baby-llama/baby-llama.cpp
examples/baby-llama/CMakeFiles/baby-llama.dir/baby-llama.cpp.o: examples/baby-llama/CMakeFiles/baby-llama.dir/compiler_depend.ts
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_1) "Building CXX object examples/baby-llama/CMakeFiles/baby-llama.dir/baby-llama.cpp.o"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/baby-llama && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -MD -MT examples/baby-llama/CMakeFiles/baby-llama.dir/baby-llama.cpp.o -MF CMakeFiles/baby-llama.dir/baby-llama.cpp.o.d -o CMakeFiles/baby-llama.dir/baby-llama.cpp.o -c /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/examples/baby-llama/baby-llama.cpp

examples/baby-llama/CMakeFiles/baby-llama.dir/baby-llama.cpp.i: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Preprocessing CXX source to CMakeFiles/baby-llama.dir/baby-llama.cpp.i"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/baby-llama && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -E /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/examples/baby-llama/baby-llama.cpp > CMakeFiles/baby-llama.dir/baby-llama.cpp.i

examples/baby-llama/CMakeFiles/baby-llama.dir/baby-llama.cpp.s: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Compiling CXX source to assembly CMakeFiles/baby-llama.dir/baby-llama.cpp.s"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/baby-llama && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -S /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/examples/baby-llama/baby-llama.cpp -o CMakeFiles/baby-llama.dir/baby-llama.cpp.s

# Object files for target baby-llama
baby__llama_OBJECTS = \
"CMakeFiles/baby-llama.dir/baby-llama.cpp.o"

# External object files for target baby-llama
baby__llama_EXTERNAL_OBJECTS = \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common/CMakeFiles/common.dir/common.cpp.o" \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common/CMakeFiles/common.dir/console.cpp.o" \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common/CMakeFiles/common.dir/grammar-parser.cpp.o"

bin/baby-llama: examples/baby-llama/CMakeFiles/baby-llama.dir/baby-llama.cpp.o
bin/baby-llama: common/CMakeFiles/common.dir/common.cpp.o
bin/baby-llama: common/CMakeFiles/common.dir/console.cpp.o
bin/baby-llama: common/CMakeFiles/common.dir/grammar-parser.cpp.o
bin/baby-llama: examples/baby-llama/CMakeFiles/baby-llama.dir/build.make
bin/baby-llama: libllama.a
bin/baby-llama: examples/baby-llama/CMakeFiles/baby-llama.dir/link.txt
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --bold --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_2) "Linking CXX executable ../../bin/baby-llama"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/baby-llama && $(CMAKE_COMMAND) -E cmake_link_script CMakeFiles/baby-llama.dir/link.txt --verbose=$(VERBOSE)

# Rule to build all files generated by this target.
examples/baby-llama/CMakeFiles/baby-llama.dir/build: bin/baby-llama
.PHONY : examples/baby-llama/CMakeFiles/baby-llama.dir/build

examples/baby-llama/CMakeFiles/baby-llama.dir/clean:
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/baby-llama && $(CMAKE_COMMAND) -P CMakeFiles/baby-llama.dir/cmake_clean.cmake
.PHONY : examples/baby-llama/CMakeFiles/baby-llama.dir/clean

examples/baby-llama/CMakeFiles/baby-llama.dir/depend:
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build && $(CMAKE_COMMAND) -E cmake_depends "Unix Makefiles" /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/examples/baby-llama /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/baby-llama /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/baby-llama/CMakeFiles/baby-llama.dir/DependInfo.cmake "--color=$(COLOR)"
.PHONY : examples/baby-llama/CMakeFiles/baby-llama.dir/depend

