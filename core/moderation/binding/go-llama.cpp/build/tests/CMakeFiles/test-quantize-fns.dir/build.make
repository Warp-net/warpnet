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
include tests/CMakeFiles/test-quantize-fns.dir/depend.make
# Include any dependencies generated by the compiler for this target.
include tests/CMakeFiles/test-quantize-fns.dir/compiler_depend.make

# Include the progress variables for this target.
include tests/CMakeFiles/test-quantize-fns.dir/progress.make

# Include the compile flags for this target's objects.
include tests/CMakeFiles/test-quantize-fns.dir/flags.make

tests/CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.o: tests/CMakeFiles/test-quantize-fns.dir/flags.make
tests/CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.o: /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/tests/test-quantize-fns.cpp
tests/CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.o: tests/CMakeFiles/test-quantize-fns.dir/compiler_depend.ts
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_1) "Building CXX object tests/CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.o"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/tests && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -MD -MT tests/CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.o -MF CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.o.d -o CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.o -c /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/tests/test-quantize-fns.cpp

tests/CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.i: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Preprocessing CXX source to CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.i"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/tests && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -E /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/tests/test-quantize-fns.cpp > CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.i

tests/CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.s: cmake_force
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green "Compiling CXX source to assembly CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.s"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/tests && /usr/bin/g++ $(CXX_DEFINES) $(CXX_INCLUDES) $(CXX_FLAGS) -S /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/tests/test-quantize-fns.cpp -o CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.s

# Object files for target test-quantize-fns
test__quantize__fns_OBJECTS = \
"CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.o"

# External object files for target test-quantize-fns
test__quantize__fns_EXTERNAL_OBJECTS = \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common/CMakeFiles/common.dir/common.cpp.o" \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common/CMakeFiles/common.dir/console.cpp.o" \
"/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/common/CMakeFiles/common.dir/grammar-parser.cpp.o"

bin/test-quantize-fns: tests/CMakeFiles/test-quantize-fns.dir/test-quantize-fns.cpp.o
bin/test-quantize-fns: common/CMakeFiles/common.dir/common.cpp.o
bin/test-quantize-fns: common/CMakeFiles/common.dir/console.cpp.o
bin/test-quantize-fns: common/CMakeFiles/common.dir/grammar-parser.cpp.o
bin/test-quantize-fns: tests/CMakeFiles/test-quantize-fns.dir/build.make
bin/test-quantize-fns: libllama.a
bin/test-quantize-fns: libllama.a
bin/test-quantize-fns: tests/CMakeFiles/test-quantize-fns.dir/link.txt
	@$(CMAKE_COMMAND) -E cmake_echo_color "--switch=$(COLOR)" --green --bold --progress-dir=/home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/CMakeFiles --progress-num=$(CMAKE_PROGRESS_2) "Linking CXX executable ../bin/test-quantize-fns"
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/tests && $(CMAKE_COMMAND) -E cmake_link_script CMakeFiles/test-quantize-fns.dir/link.txt --verbose=$(VERBOSE)

# Rule to build all files generated by this target.
tests/CMakeFiles/test-quantize-fns.dir/build: bin/test-quantize-fns
.PHONY : tests/CMakeFiles/test-quantize-fns.dir/build

tests/CMakeFiles/test-quantize-fns.dir/clean:
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/tests && $(CMAKE_COMMAND) -P CMakeFiles/test-quantize-fns.dir/cmake_clean.cmake
.PHONY : tests/CMakeFiles/test-quantize-fns.dir/clean

tests/CMakeFiles/test-quantize-fns.dir/depend:
	cd /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build && $(CMAKE_COMMAND) -E cmake_depends "Unix Makefiles" /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/llama.cpp/tests /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/tests /home/vadim/go/src/github.com/warpnet/warpnet/core/moderation/binding/go-llama.cpp/build/tests/CMakeFiles/test-quantize-fns.dir/DependInfo.cmake "--color=$(COLOR)"
.PHONY : tests/CMakeFiles/test-quantize-fns.dir/depend

