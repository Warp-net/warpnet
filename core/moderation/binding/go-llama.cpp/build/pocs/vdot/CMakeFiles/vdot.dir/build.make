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
include pocs/vdot/CMakeFiles/vdot.dir/depend.make
# Include any dependencies generated by the compiler for this target.
include pocs/vdot/CMakeFiles/vdot.dir/compiler_depend.make

# Include the progress variables for this target.
include pocs/vdot/CMakeFiles/vdot.dir/progress.make

# Include the compile flags for this target's objects.
include pocs/vdot/CMakeFiles/vdot.dir/flags.make

pocs/vdot/CMakeFiles/vdot.dir/vdot.cpp.o: pocs/vdot/CMakeFiles/vdot.dir/flags.make
pocs/vdot/CMakeFiles/vdot.dir/vdot.cpp.o: /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/pocs/vdot/vdot.cpp
pocs/vdot/CMakeFiles/vdot.dir/vdot.cpp.o: pocs/vdot/CMakeFiles/vdot.dir/compiler_depend.ts
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_1) "Building CXX object pocs/vdot/CMakeFiles/vdot.dir/vdot.cpp.o"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/pocs/vdot && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -MD -MT pocs/vdot/CMakeFiles/vdot.dir/vdot.cpp.o -MF CMakeFiles/vdot.dir/vdot.cpp.o.d -o CMakeFiles/vdot.dir/vdot.cpp.o -c /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/pocs/vdot/vdot.cpp

pocs/vdot/CMakeFiles/vdot.dir/vdot.cpp.i: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Preprocessing CXX source to CMakeFiles/vdot.dir/vdot.cpp.i"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/pocs/vdot && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -E /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/pocs/vdot/vdot.cpp > CMakeFiles/vdot.dir/vdot.cpp.i

pocs/vdot/CMakeFiles/vdot.dir/vdot.cpp.s: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Compiling CXX source to assembly CMakeFiles/vdot.dir/vdot.cpp.s"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/pocs/vdot && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -S /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/pocs/vdot/vdot.cpp -o CMakeFiles/vdot.dir/vdot.cpp.s

# Object files for target vdot
vdot_OBJECTS = \
"CMakeFiles/vdot.dir/vdot.cpp.o"

# External object files for target vdot
vdot_EXTERNAL_OBJECTS = \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common/CMakeFiles/common.dir/common.cpp.o" \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common/CMakeFiles/common.dir/console.cpp.o" \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common/CMakeFiles/common.dir/grammar-parser.cpp.o"

bin/vdot: pocs/vdot/CMakeFiles/vdot.dir/vdot.cpp.o
bin/vdot: common/CMakeFiles/common.dir/common.cpp.o
bin/vdot: common/CMakeFiles/common.dir/console.cpp.o
bin/vdot: common/CMakeFiles/common.dir/grammar-parser.cpp.o
bin/vdot: pocs/vdot/CMakeFiles/vdot.dir/build.make
bin/vdot: libllama.a
bin/vdot: pocs/vdot/CMakeFiles/vdot.dir/link.txt
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --bold --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_2) "Linking CXX executable ../../bin/vdot"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/pocs/vdot && $(CMAKE_COMMAND) -E cmake_link_script CMakeFiles/vdot.dir/link.txt --verbose=$(VERBOSE)

# Rule to build all files generated by this target.
pocs/vdot/CMakeFiles/vdot.dir/build: bin/vdot
.PHONY : pocs/vdot/CMakeFiles/vdot.dir/build

pocs/vdot/CMakeFiles/vdot.dir/clean:
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/pocs/vdot && $(CMAKE_COMMAND) -P CMakeFiles/vdot.dir/cmake_clean.cmake
.PHONY : pocs/vdot/CMakeFiles/vdot.dir/clean

pocs/vdot/CMakeFiles/vdot.dir/depend:
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build && $(CMAKE_COMMAND) -E cmake_depends "Unix Makefiles" /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/pocs/vdot /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/pocs/vdot /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/pocs/vdot/CMakeFiles/vdot.dir/DependInfo.cmake "--color=$(COLOR)"
.PHONY : pocs/vdot/CMakeFiles/vdot.dir/depend

