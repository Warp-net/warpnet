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
include examples/quantize-stats/CMakeFiles/quantize-stats.dir/depend.make
# Include any dependencies generated by the compiler for this target.
include examples/quantize-stats/CMakeFiles/quantize-stats.dir/compiler_depend.make

# Include the progress variables for this target.
include examples/quantize-stats/CMakeFiles/quantize-stats.dir/progress.make

# Include the compile flags for this target's objects.
include examples/quantize-stats/CMakeFiles/quantize-stats.dir/flags.make

examples/quantize-stats/CMakeFiles/quantize-stats.dir/quantize-stats.cpp.o: examples/quantize-stats/CMakeFiles/quantize-stats.dir/flags.make
examples/quantize-stats/CMakeFiles/quantize-stats.dir/quantize-stats.cpp.o: /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/examples/quantize-stats/quantize-stats.cpp
examples/quantize-stats/CMakeFiles/quantize-stats.dir/quantize-stats.cpp.o: examples/quantize-stats/CMakeFiles/quantize-stats.dir/compiler_depend.ts
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_1) "Building CXX object examples/quantize-stats/CMakeFiles/quantize-stats.dir/quantize-stats.cpp.o"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/quantize-stats && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -MD -MT examples/quantize-stats/CMakeFiles/quantize-stats.dir/quantize-stats.cpp.o -MF CMakeFiles/quantize-stats.dir/quantize-stats.cpp.o.d -o CMakeFiles/quantize-stats.dir/quantize-stats.cpp.o -c /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/examples/quantize-stats/quantize-stats.cpp

examples/quantize-stats/CMakeFiles/quantize-stats.dir/quantize-stats.cpp.i: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Preprocessing CXX source to CMakeFiles/quantize-stats.dir/quantize-stats.cpp.i"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/quantize-stats && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -E /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/examples/quantize-stats/quantize-stats.cpp > CMakeFiles/quantize-stats.dir/quantize-stats.cpp.i

examples/quantize-stats/CMakeFiles/quantize-stats.dir/quantize-stats.cpp.s: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Compiling CXX source to assembly CMakeFiles/quantize-stats.dir/quantize-stats.cpp.s"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/quantize-stats && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -S /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/examples/quantize-stats/quantize-stats.cpp -o CMakeFiles/quantize-stats.dir/quantize-stats.cpp.s

# Object files for target quantize-stats
quantize__stats_OBJECTS = \
"CMakeFiles/quantize-stats.dir/quantize-stats.cpp.o"

# External object files for target quantize-stats
quantize__stats_EXTERNAL_OBJECTS =

bin/quantize-stats: examples/quantize-stats/CMakeFiles/quantize-stats.dir/quantize-stats.cpp.o
bin/quantize-stats: examples/quantize-stats/CMakeFiles/quantize-stats.dir/build.make
bin/quantize-stats: libllama.a
bin/quantize-stats: examples/quantize-stats/CMakeFiles/quantize-stats.dir/link.txt
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --bold --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_2) "Linking CXX executable ../../bin/quantize-stats"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/quantize-stats && $(CMAKE_COMMAND) -E cmake_link_script CMakeFiles/quantize-stats.dir/link.txt --verbose=$(VERBOSE)

# Rule to build all files generated by this target.
examples/quantize-stats/CMakeFiles/quantize-stats.dir/build: bin/quantize-stats
.PHONY : examples/quantize-stats/CMakeFiles/quantize-stats.dir/build

examples/quantize-stats/CMakeFiles/quantize-stats.dir/clean:
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/quantize-stats && $(CMAKE_COMMAND) -P CMakeFiles/quantize-stats.dir/cmake_clean.cmake
.PHONY : examples/quantize-stats/CMakeFiles/quantize-stats.dir/clean

examples/quantize-stats/CMakeFiles/quantize-stats.dir/depend:
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build && $(CMAKE_COMMAND) -E cmake_depends "Unix Makefiles" /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/examples/quantize-stats /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/quantize-stats /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/examples/quantize-stats/CMakeFiles/quantize-stats.dir/DependInfo.cmake "--color=$(COLOR)"
.PHONY : examples/quantize-stats/CMakeFiles/quantize-stats.dir/depend

