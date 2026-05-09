package site.warpnet.warpdroid.view

import android.annotation.SuppressLint
import android.content.Context
import android.content.SharedPreferences
import android.content.res.ColorStateList
import android.graphics.drawable.RippleDrawable
import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ArrayAdapter
import android.widget.Filter
import androidx.annotation.DrawableRes
import androidx.annotation.StringRes
import androidx.appcompat.R as appcompatR
import androidx.core.content.edit
import androidx.core.graphics.ColorUtils
import androidx.core.graphics.drawable.toDrawable
import androidx.core.os.bundleOf
import androidx.fragment.app.Fragment
import androidx.fragment.app.setFragmentResult
import com.google.android.material.R as materialR
import com.google.android.material.bottomsheet.BottomSheetDialogFragment
import com.google.android.material.color.MaterialColors
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.databinding.BottomsheetConfirmationBinding
import site.warpnet.warpdroid.databinding.ItemRetweetOptionBinding
import site.warpnet.warpdroid.entity.Status
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.util.getNonNullString
import site.warpnet.warpdroid.util.getSerializableCompat
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.viewBinding
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject

@AndroidEntryPoint
class ConfirmationBottomSheet : BottomSheetDialogFragment(R.layout.bottomsheet_confirmation) {

    @Inject
    lateinit var prefs: SharedPreferences

    private val binding by viewBinding(BottomsheetConfirmationBinding::bind)

    private var selectedOption = Status.Visibility.PUBLIC

    @SuppressLint("UseCompatTextViewDrawableApis")
    override fun onViewCreated(view: View, savedInstanceState: Bundle?) {
        val mode: Mode = requireArguments().getSerializableCompat(ARG_MODE)!!
        if (mode == Mode.RETWEET) {
            selectedOption = Status.Visibility.valueOf(
                prefs.getNonNullString(PrefKeys.RETWEET_PRIVACY, Status.Visibility.PUBLIC.name)
            )

            binding.confirmTextView.setText(R.string.retweet_confirm)
            binding.confirmTextView.setCompoundDrawablesRelativeWithIntrinsicBounds(R.drawable.ic_repeat_24dp, 0, 0, 0)
            binding.confirmTextView.compoundDrawableTintList = ColorStateList.valueOf(
                MaterialColors.getColor(binding.confirmTextView, appcompatR.attr.colorPrimary)
            )

            binding.confirmButton.setText(R.string.action_retweet)

            binding.confirmButton.setOnClickListener {
                prefs.edit {
                    putString(PrefKeys.RETWEET_PRIVACY, selectedOption.name)
                }
                setFragmentResult(KEY_CONFIRM, bundleOf(RESULT_VISIBILITY to selectedOption.name))
                dismiss()
            }

            binding.retweetPrivacyDropdown.setAdapter(OptionsAdapter(view.context))

            binding.retweetPrivacyLayout.setStartIconDrawable(selectedOption.getIcon())
            binding.retweetPrivacyDropdown.setText(selectedOption.getName())

            binding.retweetPrivacyDropdown.setOnItemClickListener { _, _, position, _ ->
                selectedOption = retweetOptions.getOrElse(position) { Status.Visibility.PUBLIC }
                binding.retweetPrivacyLayout.setStartIconDrawable(selectedOption.getIcon())
                binding.retweetPrivacyDropdown.setText(selectedOption.getName())
            }
        } else {
            binding.confirmTextView.setText(R.string.like_confirm)
            binding.confirmTextView.setCompoundDrawablesRelativeWithIntrinsicBounds(R.drawable.ic_star_24dp, 0, 0, 0)
            binding.confirmTextView.compoundDrawableTintList = ColorStateList.valueOf(
                requireContext().getColor(R.color.likeButtonActiveColor)
            )

            binding.retweetPrivacyLayout.hide()

            binding.confirmButton.setText(R.string.action_like)

            binding.confirmButton.setOnClickListener {
                setFragmentResult(KEY_CONFIRM, bundleOf())
                dismiss()
            }
        }
        binding.cancelButton.setOnClickListener {
            dismiss()
        }
    }

    inner class OptionsAdapter(context: Context) : ArrayAdapter<Status.Visibility>(
        context,
        R.layout.item_retweet_option,
        retweetOptions
    ) {
        override fun getView(position: Int, convertView: View?, parent: ViewGroup): View {
            val item = getItem(position)
            val view: View = convertView ?: run {
                val layoutInflater = LayoutInflater.from(parent.context)
                val binding = ItemRetweetOptionBinding.inflate(layoutInflater)

                binding.retweetOptionName.setText(item.getName())
                binding.retweetOptionDescription.setText(item.getDescription())
                binding.retweetOptionIcon.setImageResource(item.getIcon())
                binding.root
            }
            if (item == selectedOption) {
                // using the same color as MaterialAutoCompleteTextView.MaterialArrayAdapter which is not public unfortunately
                val overlayColor = ColorUtils.setAlphaComponent(
                    MaterialColors.getColor(view, materialR.attr.colorOnSurface),
                    30
                )
                view.background = RippleDrawable(
                    ColorStateList.valueOf(overlayColor),
                    MaterialColors.getColor(view, materialR.attr.colorSecondaryContainer).toDrawable(),
                    null
                )
            } else {
                view.background = null
            }
            return view
        }

        override fun getFilter() = object : Filter() {
            override fun performFiltering(constraint: CharSequence) = FilterResults().apply { count = 3 }

            override fun publishResults(constraint: CharSequence, results: FilterResults) {
                // noop
            }
        }
    }

    enum class Mode {
        RETWEET,
        LIKE
    }

    companion object {
        private const val TAG = "ConfirmationBottomSheet"

        private const val KEY_CONFIRM = "confirm"
        private const val ARG_MODE = "mode"
        private const val RESULT_VISIBILITY = "visibility"

        private val retweetOptions = listOf(Status.Visibility.PUBLIC, Status.Visibility.UNLISTED, Status.Visibility.PRIVATE)

        fun Fragment.confirmRetweet(preferences: SharedPreferences, onConfirmed: (Status.Visibility) -> Unit) {
            if (preferences.getBoolean(PrefKeys.CONFIRM_RETWEETS, true)) {
                val bottomSheet = ConfirmationBottomSheet()
                bottomSheet.arguments = bundleOf(
                    ARG_MODE to Mode.RETWEET
                )
                bottomSheet.show(childFragmentManager, TAG)
                childFragmentManager.setFragmentResultListener(KEY_CONFIRM, this) { requestKey, result ->
                    onConfirmed(Status.Visibility.valueOf(result.getString(RESULT_VISIBILITY)!!))
                }
            } else {
                onConfirmed(Status.Visibility.PUBLIC)
            }
        }

        fun Fragment.confirmLike(preferences: SharedPreferences, onConfirmed: () -> Unit) {
            if (preferences.getBoolean(PrefKeys.CONFIRM_LIKES, false)) {
                val bottomSheet = ConfirmationBottomSheet()
                bottomSheet.arguments = bundleOf(
                    ARG_MODE to Mode.LIKE
                )
                bottomSheet.show(childFragmentManager, TAG)
                childFragmentManager.setFragmentResultListener(KEY_CONFIRM, this) { _, _ ->
                    onConfirmed()
                }
            } else {
                onConfirmed()
            }
        }

        @StringRes
        private fun Status.Visibility?.getName(): Int {
            return when (this) {
                Status.Visibility.PUBLIC -> R.string.post_privacy_public
                Status.Visibility.UNLISTED -> R.string.post_privacy_unlisted
                else -> R.string.post_privacy_followers_only
            }
        }

        @StringRes
        private fun Status.Visibility?.getDescription(): Int {
            return when (this) {
                Status.Visibility.PUBLIC -> R.string.retweet_privacy_public_description
                Status.Visibility.UNLISTED -> R.string.retweet_privacy_unlisted_description
                else -> R.string.retweet_privacy_followers_only_description
            }
        }

        @DrawableRes
        private fun Status.Visibility?.getIcon(): Int {
            return when (this) {
                Status.Visibility.PUBLIC -> R.drawable.ic_public_24dp
                Status.Visibility.UNLISTED -> R.drawable.ic_lock_open_24dp
                else -> R.drawable.ic_lock_24dp
            }
        }
    }
}
