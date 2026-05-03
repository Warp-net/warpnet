package site.warpnet.warpdroid.components.domainblocks

import android.os.Bundle
import android.util.Log
import android.view.View
import androidx.fragment.app.Fragment
import androidx.fragment.app.viewModels
import androidx.lifecycle.lifecycleScope
import androidx.paging.LoadState
import androidx.recyclerview.widget.DividerItemDecoration
import androidx.recyclerview.widget.LinearLayoutManager
import com.google.android.material.snackbar.Snackbar
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.FragmentDomainBlocksBinding
import site.warpnet.warpdroid.util.ensureBottomPadding
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.show
import site.warpnet.warpdroid.util.viewBinding
import site.warpnet.warpdroid.util.visible
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch

@AndroidEntryPoint
class DomainBlocksFragment : Fragment(R.layout.fragment_domain_blocks) {

    private val binding by viewBinding(FragmentDomainBlocksBinding::bind)

    private val viewModel: DomainBlocksViewModel by viewModels()

    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        val adapter = DomainBlocksAdapter(viewModel::unblock)

        binding.recyclerView.ensureBottomPadding()

        binding.recyclerView.setHasFixedSize(true)
        binding.recyclerView.addItemDecoration(
            DividerItemDecoration(view.context, DividerItemDecoration.VERTICAL)
        )
        binding.recyclerView.adapter = adapter
        binding.recyclerView.layoutManager = LinearLayoutManager(view.context)

        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.uiEvents.collect { event ->
                showSnackbar(event)
            }
        }

        viewLifecycleOwner.lifecycleScope.launch {
            viewModel.domainPager.collectLatest { pagingData ->
                adapter.submitData(pagingData)
            }
        }

        adapter.addLoadStateListener { loadState ->
            binding.progressBar.visible(
                loadState.refresh == LoadState.Loading && adapter.itemCount == 0
            )

            if (loadState.refresh is LoadState.Error) {
                binding.recyclerView.hide()
                binding.messageView.show()
                val errorState = loadState.refresh as LoadState.Error
                binding.messageView.setup(errorState.error) { adapter.retry() }
                Log.w(TAG, "error loading blocked domains", errorState.error)
            } else if (loadState.refresh is LoadState.NotLoading && adapter.itemCount == 0) {
                binding.recyclerView.hide()
                binding.messageView.show()
                binding.messageView.setup(R.drawable.elephant_friend_empty, R.string.message_empty)
            } else {
                binding.recyclerView.show()
                binding.messageView.hide()
            }
        }
    }

    private fun showSnackbar(event: SnackbarEvent) {
        val message = if (event.throwable == null) {
            getString(event.message, event.domain)
        } else {
            Log.w(TAG, event.throwable)
            val error = event.throwable.localizedMessage ?: getString(R.string.ui_error_unknown)
            getString(event.message, event.domain, error)
        }

        Snackbar.make(binding.recyclerView, message, Snackbar.LENGTH_LONG)
            .setTextMaxLines(5)
            .setAction(event.actionText, event.action)
            .show()
    }

    companion object {
        private const val TAG = "DomainBlocksFragment"
    }
}
