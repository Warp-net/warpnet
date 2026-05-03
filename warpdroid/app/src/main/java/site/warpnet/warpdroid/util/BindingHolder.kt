package site.warpnet.warpdroid.util

import androidx.recyclerview.widget.RecyclerView
import androidx.viewbinding.ViewBinding

class BindingHolder<T : ViewBinding>(
    val binding: T
) : RecyclerView.ViewHolder(binding.root)
