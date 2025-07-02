file(REMOVE_RECURSE
  "libggml_static.a"
  "libggml_static.pdb"
)

# Per-language clean rules from dependency scanning.
foreach(lang C)
  include(CMakeFiles/ggml_static.dir/cmake_clean_${lang}.cmake OPTIONAL)
endforeach()
