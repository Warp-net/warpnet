/* Copyright 2019 Warpdroid Contributors
 *
 * This file is a part of Warpdroid.
 *
 * This program is free software; you can redistribute it and/or modify it under the terms of the
 * GNU General Public License as published by the Free Software Foundation; either version 3 of the
 * License, or (at your option) any later version.
 *
 * Warpdroid is distributed in the hope that it will be useful, but WITHOUT ANY WARRANTY; without even
 * the implied warranty of MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
 * Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with Warpdroid; if not,
 * see <http://www.gnu.org/licenses>. */

package site.warpnet.warpdroid.components.compose

import android.Manifest
import android.content.ClipData
import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import android.graphics.Bitmap
import android.icu.text.BreakIterator
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Parcelable
import android.provider.MediaStore
import android.text.Spanned
import android.text.style.URLSpan
import android.util.Log
import android.view.KeyEvent
import android.view.MenuItem
import android.view.View
import android.view.ViewGroup
import android.widget.AdapterView
import android.widget.ImageButton
import android.widget.LinearLayout
import android.widget.PopupMenu
import android.widget.Toast
import androidx.activity.OnBackPressedCallback
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.annotation.AttrRes
import androidx.annotation.ColorInt
import androidx.annotation.StringRes
import androidx.annotation.VisibleForTesting
import androidx.appcompat.R as appcompatR
import androidx.appcompat.content.res.AppCompatResources
import androidx.core.content.FileProvider
import androidx.core.content.res.use
import androidx.core.view.ContentInfoCompat
import androidx.core.view.OnReceiveContentListener
import androidx.core.view.WindowInsetsCompat.Type.ime
import androidx.core.view.WindowInsetsCompat.Type.systemBars
import androidx.core.view.isGone
import androidx.core.view.isVisible
import androidx.core.view.updateLayoutParams
import androidx.core.view.updatePadding
import androidx.core.widget.doAfterTextChanged
import androidx.core.widget.doOnTextChanged
import androidx.lifecycle.lifecycleScope
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.transition.TransitionManager
import com.google.android.material.bottomsheet.BottomSheetBehavior
import com.google.android.material.bottomsheet.BottomSheetBehavior.BottomSheetCallback
import com.google.android.material.color.MaterialColors
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.google.android.material.snackbar.Snackbar
import site.warpnet.warpdroid.BaseActivity
import site.warpnet.warpdroid.BuildConfig
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.adapter.LocaleAdapter
import site.warpnet.warpdroid.components.compose.ComposeViewModel.ConfirmationKind
import site.warpnet.warpdroid.components.compose.ComposeViewModel.QueuedMedia
import site.warpnet.warpdroid.components.compose.dialog.CaptionDialog
import site.warpnet.warpdroid.components.compose.dialog.makeFocusDialog
import site.warpnet.warpdroid.components.compose.view.ComposeOptionsListener
import site.warpnet.warpdroid.components.editimage.EditImageContract
import site.warpnet.warpdroid.components.editimage.EditImageOptions
import site.warpnet.warpdroid.components.editimage.EditImageResult
import site.warpnet.transport.WarpnetLimits
import site.warpnet.warpdroid.components.instanceinfo.InstanceInfoRepository
import site.warpnet.warpdroid.databinding.ActivityComposeBinding
import site.warpnet.warpdroid.db.entity.AccountEntity
import site.warpnet.warpdroid.entity.Attachment
import site.warpnet.warpdroid.entity.Tweet
import site.warpnet.warpdroid.settings.AppTheme
import site.warpnet.warpdroid.settings.PrefKeys
import site.warpnet.warpdroid.settings.PrefKeys.APP_THEME
import site.warpnet.warpdroid.util.MentionSpan
import site.warpnet.warpdroid.util.PickMediaFiles
import site.warpnet.warpdroid.util.defaultFinders
import site.warpnet.warpdroid.util.getInitialLanguages
import site.warpnet.warpdroid.util.getLocaleList
import site.warpnet.warpdroid.util.getMediaSize
import site.warpnet.warpdroid.util.getParcelableArrayListExtraCompat
import site.warpnet.warpdroid.util.getParcelableCompat
import site.warpnet.warpdroid.util.getParcelableExtraCompat
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.highlightSpans
import site.warpnet.warpdroid.util.loadAvatar
import site.warpnet.warpdroid.util.map
import site.warpnet.warpdroid.util.modernLanguageCode
import site.warpnet.warpdroid.util.setDrawableTint
import site.warpnet.warpdroid.util.setOnWindowInsetsChangeListener
import site.warpnet.warpdroid.util.show
import site.warpnet.warpdroid.util.viewBinding
import site.warpnet.warpdroid.util.visible
import dagger.hilt.android.AndroidEntryPoint
import dagger.hilt.android.lifecycle.withCreationCallback
import dagger.hilt.android.migration.OptionalInject
import java.io.File
import java.io.IOException
import java.text.DecimalFormat
import java.util.Locale
import kotlin.math.max
import kotlin.math.min
import kotlinx.coroutines.flow.collect
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.parcelize.Parcelize

@OptionalInject
@AndroidEntryPoint
class ComposeActivity :
    BaseActivity(),
    ComposeOptionsListener,
    ComposeAutoCompleteAdapter.AutocompletionProvider,
    OnReceiveContentListener,
    CaptionDialog.Listener {

    private lateinit var composeOptionsBehavior: BottomSheetBehavior<*>
    private lateinit var addMediaBehavior: BottomSheetBehavior<*>

    /** The account that is being used to compose the status */
    private lateinit var activeAccount: AccountEntity

    private var photoUploadUri: Uri? = null

    @VisibleForTesting
    var highlightFinders = defaultFinders

    @VisibleForTesting
    var maximumTootCharacters = WarpnetLimits.MAX_TWEET_CHARS
    var charactersReservedPerUrl = InstanceInfoRepository.DEFAULT_CHARACTERS_RESERVED_PER_URL

    private val viewModel: ComposeViewModel by viewModels(
        extrasProducer = {
            defaultViewModelCreationExtras.withCreationCallback<ComposeViewModel.Factory> { factory ->
                factory.create(
                    options = intent.getParcelableExtraCompat(COMPOSE_OPTIONS_EXTRA),
                )
            }
        }
    )

    private val binding by viewBinding(ActivityComposeBinding::inflate)

    private var maxUploadMediaNumber = InstanceInfoRepository.DEFAULT_MAX_MEDIA_ATTACHMENTS
    private var mediaDescriptionLimit = InstanceInfoRepository.DEFAULT_MEDIA_DESCRIPTION_LIMIT

    private val takePictureLauncher =
        registerForActivityResult(ActivityResultContracts.TakePicture()) { success ->
            if (success) {
                viewModel.pickMedia(photoUploadUri!!)
            }
        }
    private val pickMediaFilePermissionLauncher =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) { isGranted ->
            if (isGranted) {
                pickMediaFileLauncher.launch(true)
            } else {
                Snackbar.make(
                    binding.activityCompose,
                    R.string.error_media_upload_permission,
                    Snackbar.LENGTH_SHORT
                ).apply {
                    setAction(R.string.action_retry) { onMediaPick() }
                    // necessary so snackbar is shown over everything
                    view.elevation = resources.getDimension(R.dimen.compose_activity_snackbar_elevation)
                    show()
                }
            }
        }
    private val pickMediaFileLauncher = registerForActivityResult(PickMediaFiles()) { uris ->
        if (viewModel.media.value.size + uris.size > maxUploadMediaNumber) {
            Toast.makeText(
                this,
                resources.getQuantityString(
                    R.plurals.error_upload_max_media_reached,
                    maxUploadMediaNumber,
                    maxUploadMediaNumber
                ),
                Toast.LENGTH_SHORT
            ).show()
        } else {
            viewModel.pickMedia(
                uris.map { uri ->
                    ComposeViewModel.MediaData(uri)
                }
            )
        }
    }

    // Contract kicked off by editImageInQueue; expects viewModel.cropImageItemOld set
    private val editImage = registerForActivityResult(EditImageContract()) { result ->

        when (result) {
            is EditImageResult.Success -> {
                viewModel.cropImageItemOld?.let { itemOld ->
                    val size = getMediaSize(contentResolver, result.outputUri)

                    viewModel.addMediaToQueue(
                        type = itemOld.type,
                        uri = result.outputUri,
                        mediaSize = size,
                        description = itemOld.description,
                        // Intentionally reset focus when cropping
                        focus = null,
                        replaceItem = itemOld
                    )
                }
            }
            is EditImageResult.Error -> {
                Log.w(TAG, "Edit image failed: " + result.exception)
                displayTransientMessage(R.string.error_image_edit_failed)
            }
            is EditImageResult.Cancelled -> {
                Log.w(TAG, "Edit image cancelled by user")
            }
        }
        viewModel.cropImageItemOld = null
    }

    private val onBackPressedCallback = object : OnBackPressedCallback(false) {
        override fun handleOnBackPressed() {
            if (composeOptionsBehavior.state == BottomSheetBehavior.STATE_EXPANDED ||
                addMediaBehavior.state == BottomSheetBehavior.STATE_EXPANDED
            ) {
                composeOptionsBehavior.state = BottomSheetBehavior.STATE_HIDDEN
                addMediaBehavior.state = BottomSheetBehavior.STATE_HIDDEN
                return
            }

            handleCloseButton()
        }
    }

    public override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        activeAccount = accountManager.activeAccount ?: return

        val theme = preferences.getString(APP_THEME, AppTheme.DEFAULT.value)
        if (theme == "black") {
            setTheme(R.style.WarpdroidDialogActivityBlackTheme)
        }
        setContentView(binding.root)

        binding.composeBottomBar.setOnWindowInsetsChangeListener { windowInsets ->
            val insets = windowInsets.getInsets(systemBars() or ime())
            val bottomBarHeight = resources.getDimensionPixelSize(R.dimen.compose_bottom_bar_height)
            val bottomBarPadding = resources.getDimensionPixelSize(R.dimen.compose_bottom_bar_padding_vertical)
            binding.composeBottomBar.updatePadding(bottom = insets.bottom + bottomBarPadding)
            binding.addMediaBottomSheet.updatePadding(bottom = insets.bottom + bottomBarHeight)
            binding.composeOptionsBottomSheet.updatePadding(bottom = insets.bottom + bottomBarHeight)
            binding.composeMainScrollView.updateLayoutParams<ViewGroup.MarginLayoutParams> { bottomMargin = insets.bottom + bottomBarHeight }
        }

        setupActionBar()

        setupAvatar(activeAccount)
        val mediaAdapter = MediaPreviewAdapter(
            this,
            onAddCaption = { item ->
                CaptionDialog.newInstance(
                    item.localId,
                    item.description,
                    item.uri,
                    mediaDescriptionLimit
                ).show(supportFragmentManager, "caption_dialog")
            },
            onAddFocus = { item ->
                makeFocusDialog(item.focus, item.uri) { newFocus ->
                    viewModel.updateFocus(item.localId, newFocus)
                }
                // TODO this is inconsistent to CaptionDialog (device rotation)?
            },
            onEditImage = this::editImageInQueue,
            onRemove = this::removeMediaFromQueue
        )
        binding.composeMediaPreviewBar.layoutManager =
            LinearLayoutManager(this, LinearLayoutManager.HORIZONTAL, false)
        binding.composeMediaPreviewBar.adapter = mediaAdapter
        binding.composeMediaPreviewBar.itemAnimator = null

        /* If the composer is started up as a reply to another post, override the "starting" state
         * based on what the intent from the reply request passes. */
        val composeOptions: ComposeOptions? = intent.getParcelableExtraCompat(COMPOSE_OPTIONS_EXTRA)

        setupButtons()
        subscribeToUpdates(mediaAdapter)

        if (accountManager.shouldDisplaySelfUsername()) {
            binding.composeUsernameView.text = getString(
                R.string.compose_active_account_description,
                activeAccount.fullName
            )
            binding.composeUsernameView.show()
        } else {
            binding.composeUsernameView.hide()
        }

        setupReplyViews(composeOptions?.replyingStatusAuthor, composeOptions?.replyingTweetContent)
        val tweetContent = composeOptions?.content
        if (!tweetContent.isNullOrEmpty()) {
            binding.composeEditField.setText(tweetContent)
        }

        setupLanguageSpinner(getInitialLanguages(composeOptions?.language, activeAccount))
        setupComposeField(preferences, viewModel.startingText)
        setupContentWarningField(composeOptions?.contentWarning)
        applyShareIntent(intent, savedInstanceState)

        /* Finally, overwrite state with data from saved instance state. */
        savedInstanceState?.let {
            photoUploadUri = it.getParcelableCompat(PHOTO_UPLOAD_URI_KEY)
        }

        binding.composeEditField.post {
            binding.composeEditField.requestFocus()
        }
    }

    private fun applyShareIntent(intent: Intent, savedInstanceState: Bundle?) {
        if (savedInstanceState == null) {
            /* Get incoming images being sent through a share action from another app. Only do this
             * when savedInstanceState is null, otherwise both the images from the intent and the
             * instance state will be re-queued. */
            intent.type?.also { type ->
                if (type.startsWith("image/") || type.startsWith("video/") || type.startsWith("audio/")) {
                    when (intent.action) {
                        Intent.ACTION_SEND -> {
                            intent.getParcelableExtraCompat<Uri>(Intent.EXTRA_STREAM)?.let { uri ->
                                viewModel.pickMedia(uri)
                            }
                        }
                        Intent.ACTION_SEND_MULTIPLE -> {
                            intent.getParcelableArrayListExtraCompat<Uri>(Intent.EXTRA_STREAM)
                                ?.map { uri ->
                                    ComposeViewModel.MediaData(uri)
                                }?.let(viewModel::pickMedia)
                        }
                    }
                }

                val subject = intent.getStringExtra(Intent.EXTRA_SUBJECT)
                val text = intent.getStringExtra(Intent.EXTRA_TEXT).orEmpty()
                val shareBody = if (!subject.isNullOrBlank() && subject !in text) {
                    subject + '\n' + text
                } else {
                    text
                }

                if (shareBody.isNotBlank()) {
                    val start = binding.composeEditField.selectionStart.coerceAtLeast(0)
                    val end = binding.composeEditField.selectionEnd.coerceAtLeast(0)
                    val left = min(start, end)
                    val right = max(start, end)
                    binding.composeEditField.text.replace(
                        left,
                        right,
                        shareBody,
                        0,
                        shareBody.length
                    )
                    // move edittext cursor to first when shareBody parsed
                    binding.composeEditField.text.insert(0, "\n")
                    binding.composeEditField.setSelection(0)
                }
            }
        }
    }

    private fun setupReplyViews(replyingStatusAuthor: String?, replyingTweetContent: String?) {
        if (replyingStatusAuthor != null) {
            binding.composeReplyView.show()
            binding.composeReplyView.text = getString(R.string.replying_to, replyingStatusAuthor)
            val arrowDownIcon = AppCompatResources.getDrawable(this, R.drawable.ic_arrow_drop_down_24dp)!!

            setDrawableTint(this, arrowDownIcon, android.R.attr.textColorTertiary)
            binding.composeReplyView.setCompoundDrawablesRelativeWithIntrinsicBounds(
                null,
                null,
                arrowDownIcon,
                null
            )

            binding.composeReplyView.setOnClickListener {
                TransitionManager.beginDelayedTransition(
                    binding.composeReplyContentView.parent as ViewGroup
                )

                if (binding.composeReplyContentView.isVisible) {
                    binding.composeReplyContentView.hide()
                    binding.composeReplyView.setCompoundDrawablesRelativeWithIntrinsicBounds(
                        null,
                        null,
                        arrowDownIcon,
                        null
                    )
                } else {
                    binding.composeReplyContentView.show()
                    val arrowUpIcon = AppCompatResources.getDrawable(this, R.drawable.ic_arrow_drop_up_24dp)!!

                    setDrawableTint(this, arrowUpIcon, android.R.attr.textColorTertiary)
                    binding.composeReplyView.setCompoundDrawablesRelativeWithIntrinsicBounds(
                        null,
                        null,
                        arrowUpIcon,
                        null
                    )
                }
            }
        }
        replyingTweetContent?.let { binding.composeReplyContentView.text = it }
    }

    private fun setupContentWarningField(startingContentWarning: String?) {
        if (startingContentWarning != null) {
            binding.composeContentWarningField.setText(startingContentWarning)
        }
        binding.composeContentWarningField.doOnTextChanged { newContentWarning, _, _, _ ->
            updateVisibleCharactersLeft()
            viewModel.updateContentWarning(newContentWarning?.toString())
        }
    }

    private fun setupComposeField(preferences: SharedPreferences, startingText: String?) {
        binding.composeEditField.setOnReceiveContentListener(this)

        binding.composeEditField.setOnKeyListener { _, keyCode, event ->
            this.onKeyDown(
                keyCode,
                event
            )
        }

        binding.composeEditField.setAdapter(
            ComposeAutoCompleteAdapter(
                this,
                animateAvatar = preferences.getBoolean(PrefKeys.ANIMATE_GIF_AVATARS, false),
                animateEmojis = preferences.getBoolean(PrefKeys.ANIMATE_CUSTOM_EMOJIS, false),
                showBotBadge = preferences.getBoolean(PrefKeys.SHOW_BOT_OVERLAY, true)
            )
        )
        binding.composeEditField.setTokenizer(ComposeTokenizer())

        binding.composeEditField.setText(startingText)
        binding.composeEditField.setSelection(binding.composeEditField.length())

        val mentionColour = binding.composeEditField.linkTextColors.defaultColor
        binding.composeEditField.text.highlightSpans(mentionColour, highlightFinders)
        binding.composeEditField.doAfterTextChanged { editable ->
            editable!!.highlightSpans(mentionColour, highlightFinders)
            updateVisibleCharactersLeft()
            viewModel.updateContent(editable.toString())
        }

        // work around Android platform bug -> https://issuetracker.google.com/issues/67102093
        if (Build.VERSION.SDK_INT == Build.VERSION_CODES.O ||
            Build.VERSION.SDK_INT == Build.VERSION_CODES.O_MR1
        ) {
            binding.composeEditField.setLayerType(View.LAYER_TYPE_SOFTWARE, null)
        }
    }

    private fun subscribeToUpdates(mediaAdapter: MediaPreviewAdapter) {
        lifecycleScope.launch {
            viewModel.instanceInfo.collect { instanceData ->
                maximumTootCharacters = instanceData.maxChars
                charactersReservedPerUrl = instanceData.charactersReservedPerUrl
                maxUploadMediaNumber = instanceData.maxMediaAttachments
                mediaDescriptionLimit = instanceData.mediaDescriptionLimit
                updateVisibleCharactersLeft()
            }
        }

        lifecycleScope.launch {
            viewModel.showContentWarning.combine(
                viewModel.markMediaAsSensitive
            ) { showContentWarning, markSensitive ->
                updateSensitiveMediaToggle(markSensitive, showContentWarning)
                showContentWarning(showContentWarning)
            }.collect()
        }

        lifecycleScope.launch {
            viewModel.statusVisibility.collect(::setStatusVisibility)
        }

        lifecycleScope.launch {
            viewModel.media.collect { media ->
                mediaAdapter.submitList(media)

                binding.composeMediaPreviewBar.visible(media.isNotEmpty())
                updateSensitiveMediaToggle(
                    viewModel.markMediaAsSensitive.value,
                    viewModel.showContentWarning.value
                )
            }
        }

        lifecycleScope.launch {
            viewModel.media.collect { media ->
                val active = media.size < maxUploadMediaNumber &&
                    (media.isEmpty() || media.first().type == QueuedMedia.Type.IMAGE)
                enableButton(binding.composeAddMediaButton, active, active)
            }
        }

        lifecycleScope.launch {
            viewModel.uploadError.collect { throwable ->
                val errorString = when (throwable) {
                    is UploadServerError -> throwable.errorMessage
                    is FileSizeException -> {
                        val decimalFormat = DecimalFormat("0.##")
                        val allowedSizeInMb = throwable.allowedSizeInBytes.toDouble() / (1024 * 1024)
                        val formattedSize = decimalFormat.format(allowedSizeInMb)
                        getString(R.string.error_multimedia_size_limit, formattedSize)
                    }
                    is VideoOrImageException -> getString(
                        R.string.error_media_upload_image_or_video
                    )
                    is CouldNotOpenFileException -> getString(R.string.error_media_upload_opening)
                    is MediaTypeException -> getString(R.string.error_media_upload_opening)
                    else -> getString(
                        R.string.error_media_upload_sending_fmt,
                        throwable.message
                    )
                }
                displayTransientMessage(errorString)
            }
        }

        lifecycleScope.launch {
            viewModel.closeConfirmation.collect {
                updateOnBackPressedCallbackState()
            }
        }
    }

    private fun setupButtons() {
        binding.composeOptionsBottomSheet.listener = this

        composeOptionsBehavior = BottomSheetBehavior.from(binding.composeOptionsBottomSheet)
        addMediaBehavior = BottomSheetBehavior.from(binding.addMediaBottomSheet)

        composeOptionsBehavior.state = BottomSheetBehavior.STATE_HIDDEN
        addMediaBehavior.state = BottomSheetBehavior.STATE_HIDDEN

        val bottomSheetCallback = object : BottomSheetCallback() {
            override fun onStateChanged(bottomSheet: View, newState: Int) {
                updateOnBackPressedCallbackState()
            }
            override fun onSlide(bottomSheet: View, slideOffset: Float) { }
        }
        composeOptionsBehavior.addBottomSheetCallback(bottomSheetCallback)
        addMediaBehavior.addBottomSheetCallback(bottomSheetCallback)

        // Setup the interface buttons.
        binding.composeTweetButton.setOnClickListener { onSendClicked() }
        binding.composeAddMediaButton.setOnClickListener { openPickDialog() }
        binding.composeToggleVisibilityButton.setOnClickListener { showComposeOptions() }
        binding.composeContentWarningButton.setOnClickListener { onContentWarningChanged() }
        binding.composeHideMediaButton.setOnClickListener { toggleHideMedia() }
        binding.atButton.setOnClickListener { atButtonClicked() }
        binding.hashButton.setOnClickListener { hashButtonClicked() }
        binding.descriptionMissingWarningButton.setOnClickListener {
            displayTransientMessage(R.string.hint_media_description_missing)
        }

        binding.actionPhotoTake.visible(
            Intent(MediaStore.ACTION_IMAGE_CAPTURE).resolveActivity(packageManager) != null
        )

        binding.actionPhotoTake.setOnClickListener { initiateCameraApp() }
        binding.actionPhotoPick.setOnClickListener { onMediaPick() }

        onBackPressedDispatcher.addCallback(this, onBackPressedCallback)
    }

    private fun setupLanguageSpinner(initialLanguages: List<String>) {
        binding.composePostLanguageButton.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(
                parent: AdapterView<*>,
                view: View?,
                position: Int,
                id: Long
            ) {
                viewModel.postLanguage = (parent.adapter.getItem(position) as Locale).modernLanguageCode
            }

            override fun onNothingSelected(parent: AdapterView<*>) {
                parent.setSelection(0)
            }
        }
        binding.composePostLanguageButton.apply {
            adapter = LocaleAdapter(context, android.R.layout.simple_spinner_dropdown_item, getLocaleList(initialLanguages))
            setSelection(0)
        }
    }

    private fun setupActionBar() {
        setSupportActionBar(binding.toolbar)
        supportActionBar?.run {
            title = null
            setDisplayHomeAsUpEnabled(true)
            setDisplayShowHomeEnabled(true)
            setHomeAsUpIndicator(R.drawable.ic_close_24dp)
        }
    }

    private fun setupAvatar(activeAccount: AccountEntity) {
        val actionBarSizeAttr = intArrayOf(androidx.appcompat.R.attr.actionBarSize)
        val avatarSize = obtainStyledAttributes(null, actionBarSizeAttr).use { a ->
            a.getDimensionPixelSize(0, 1)
        }

        val animateAvatars = preferences.getBoolean(PrefKeys.ANIMATE_GIF_AVATARS, false)
        loadAvatar(
            activeAccount.profilePictureUrl,
            binding.composeAvatar,
            avatarSize / 8,
            animateAvatars
        )
        binding.composeAvatar.contentDescription = getString(
            R.string.compose_active_account_description,
            activeAccount.fullName
        )
    }

    private fun updateOnBackPressedCallbackState() {
        val confirmation = viewModel.closeConfirmation.value
        onBackPressedCallback.isEnabled = confirmation != ConfirmationKind.NONE ||
            composeOptionsBehavior.state != BottomSheetBehavior.STATE_HIDDEN ||
            addMediaBehavior.state != BottomSheetBehavior.STATE_HIDDEN
    }

    fun prependSelectedWordsWith(text: CharSequence) {
        // If you select "backward" in an editable, you get SelectionStart > SelectionEnd
        val start = binding.composeEditField.selectionStart.coerceAtMost(
            binding.composeEditField.selectionEnd
        )
        val end = binding.composeEditField.selectionStart.coerceAtLeast(
            binding.composeEditField.selectionEnd
        )
        val editorText = binding.composeEditField.text

        if (start == end) {
            // No selection, just insert text at caret
            editorText.insert(start, text)
            // Set the cursor after the inserted text
            binding.composeEditField.setSelection(start + text.length)
        } else {
            var wasWord: Boolean
            var isWord = end < editorText.length && !Character.isWhitespace(editorText[end])
            var newEnd = end

            // Iterate the selection backward so we don't have to juggle indices on insertion
            var index = end - 1
            while (index >= start - 1 && index >= 0) {
                wasWord = isWord
                isWord = !Character.isWhitespace(editorText[index])
                if (wasWord && !isWord) {
                    // We've reached the beginning of a word, perform insert
                    editorText.insert(index + 1, text)
                    newEnd += text.length
                }
                --index
            }

            if (start == 0 && isWord) {
                // Special case when the selection includes the start of the text
                editorText.insert(0, text)
                newEnd += text.length
            }

            // Keep the same text (including insertions) selected
            binding.composeEditField.setSelection(start, newEnd)
        }
    }

    private fun atButtonClicked() {
        prependSelectedWordsWith("@")
    }

    private fun hashButtonClicked() {
        prependSelectedWordsWith("#")
    }

    override fun onSaveInstanceState(outState: Bundle) {
        outState.putParcelable(PHOTO_UPLOAD_URI_KEY, photoUploadUri)

        super.onSaveInstanceState(outState)
    }

    private fun displayTransientMessage(message: String) {
        val bar = Snackbar.make(binding.activityCompose, message, Snackbar.LENGTH_LONG)
        // necessary so snackbar is shown over everything
        bar.view.elevation = resources.getDimension(R.dimen.compose_activity_snackbar_elevation)
        bar.setAnchorView(R.id.composeBottomBar)
        bar.show()
    }
    private fun displayTransientMessage(@StringRes stringId: Int) {
        displayTransientMessage(getString(stringId))
    }

    private fun toggleHideMedia() {
        this.viewModel.toggleMarkSensitive()
    }

    private fun updateSensitiveMediaToggle(
        markMediaSensitive: Boolean,
        contentWarningShown: Boolean
    ) {
        if (viewModel.media.value.isEmpty()) {
            binding.composeHideMediaButton.hide()
            binding.descriptionMissingWarningButton.hide()
        } else {
            binding.composeHideMediaButton.show()
            @AttrRes val color = if (contentWarningShown) {
                binding.composeHideMediaButton.setImageResource(R.drawable.ic_visibility_off_24dp)
                binding.composeHideMediaButton.isClickable = false
                appcompatR.attr.colorPrimary
            } else {
                binding.composeHideMediaButton.isClickable = true
                if (markMediaSensitive) {
                    binding.composeHideMediaButton.setImageResource(R.drawable.ic_visibility_off_24dp)
                    appcompatR.attr.colorPrimary
                } else {
                    binding.composeHideMediaButton.setImageResource(R.drawable.ic_visibility_24dp)
                    android.R.attr.textColorTertiary
                }
            }
            binding.composeHideMediaButton.drawable.setTint(
                MaterialColors.getColor(
                    binding.composeHideMediaButton,
                    color
                )
            )

            val oneMediaWithoutDescription = viewModel.media.value.any { media ->
                media.description.isNullOrEmpty()
            }
            binding.descriptionMissingWarningButton.visibility = if (oneMediaWithoutDescription) View.VISIBLE else View.GONE
        }
    }

    private fun enableButtons(enable: Boolean, editing: Boolean) {
        binding.composeAddMediaButton.isClickable = enable
        binding.composeToggleVisibilityButton.isClickable = enable && !editing
        binding.composeHideMediaButton.isClickable = enable
        binding.composeTweetButton.isEnabled = enable
    }

    private fun setStatusVisibility(visibility: Tweet.Visibility) {
        binding.composeOptionsBottomSheet.setStatusVisibility(visibility)
        binding.composeTweetButton.setStatusVisibility(visibility)

        val iconRes = when (visibility) {
            Tweet.Visibility.PUBLIC -> R.drawable.ic_public_24dp
            Tweet.Visibility.PRIVATE -> R.drawable.ic_lock_24dp
            Tweet.Visibility.DIRECT -> R.drawable.ic_mail_24dp
            Tweet.Visibility.UNLISTED -> R.drawable.ic_lock_open_24dp
            else -> R.drawable.ic_lock_open_24dp
        }
        binding.composeToggleVisibilityButton.setImageResource(iconRes)
        if (viewModel.editing) {
            // Can't update visibility on published status
            enableButton(
                binding.composeToggleVisibilityButton,
                clickable = false,
                colorActive = false
            )
        }
    }

    private fun showComposeOptions() {
        if (composeOptionsBehavior.state == BottomSheetBehavior.STATE_HIDDEN || composeOptionsBehavior.state == BottomSheetBehavior.STATE_COLLAPSED) {
            composeOptionsBehavior.state = BottomSheetBehavior.STATE_EXPANDED
            addMediaBehavior.state = BottomSheetBehavior.STATE_HIDDEN
        } else {
            composeOptionsBehavior.setState(BottomSheetBehavior.STATE_HIDDEN)
        }
    }

    private fun openPickDialog() {
        if (addMediaBehavior.state == BottomSheetBehavior.STATE_HIDDEN || addMediaBehavior.state == BottomSheetBehavior.STATE_COLLAPSED) {
            addMediaBehavior.state = BottomSheetBehavior.STATE_EXPANDED
            composeOptionsBehavior.state = BottomSheetBehavior.STATE_HIDDEN
        } else {
            addMediaBehavior.setState(BottomSheetBehavior.STATE_HIDDEN)
        }
    }

    private fun onMediaPick() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) {
            pickMediaFilePermissionLauncher.launch(Manifest.permission.READ_EXTERNAL_STORAGE)
        } else {
            pickMediaFileLauncher.launch(true)
        }
        addMediaBehavior.setState(BottomSheetBehavior.STATE_HIDDEN)
    }

    override fun onVisibilityChanged(visibility: Tweet.Visibility) {
        composeOptionsBehavior.state = BottomSheetBehavior.STATE_HIDDEN
        viewModel.changeStatusVisibility(visibility)
    }

    @VisibleForTesting
    fun calculateTextLength(): Int {
        return statusLength(
            binding.composeEditField.text,
            binding.composeContentWarningField.text,
            charactersReservedPerUrl
        )
    }

    @VisibleForTesting
    val selectedLanguage: String?
        get() = viewModel.postLanguage

    private fun updateVisibleCharactersLeft() {
        val remainingLength = maximumTootCharacters - calculateTextLength()
        binding.composeCharactersLeftView.text = String.format(Locale.getDefault(), "%d", remainingLength)

        val textColor = if (remainingLength < 0) {
            getColor(R.color.warning_color)
        } else {
            MaterialColors.getColor(
                binding.composeCharactersLeftView,
                android.R.attr.textColorTertiary
            )
        }
        binding.composeCharactersLeftView.setTextColor(textColor)
    }

    private fun onContentWarningChanged() {
        val showWarning = binding.composeContentWarningBar.isGone
        viewModel.contentWarningChanged(showWarning)
        updateVisibleCharactersLeft()
    }

    private fun onSendClicked() {
        sendStatus()
    }

    /** This is for the fancy keyboards which can insert images and stuff, and drag&drop etc */
    override fun onReceiveContent(view: View, contentInfo: ContentInfoCompat): ContentInfoCompat? {
        if (contentInfo.clip.description.hasMimeType("image/*")) {
            val split = contentInfo.partition { item: ClipData.Item -> item.uri != null }
            split.first?.let { content ->
                val description = (contentInfo.clip.description.label as String?)?.let {
                    // The Gboard android keyboard attaches this text whenever the user
                    // pastes something from the keyboard's suggestion bar.
                    // Due to different end user locales, the exact text may vary, but at
                    // least in version 13.4.08, all of the translations contained the
                    // string "Gboard".
                    if ("Gboard" in it) {
                        null
                    } else {
                        it
                    }
                }

                viewModel.pickMedia(
                    content.clip.map { clipItem ->
                        ComposeViewModel.MediaData(
                            uri = clipItem.uri,
                            description = description
                        )
                    }
                )
            }
            return split.second
        }
        return contentInfo
    }

    private fun sendStatus() {
        enableButtons(false, viewModel.editing)
        val contentText = binding.composeEditField.text.toString()
        var spoilerText = ""
        if (viewModel.showContentWarning.value) {
            spoilerText = binding.composeContentWarningField.text.toString()
        }
        val characterCount = calculateTextLength()
        if ((characterCount <= 0 || contentText.isBlank()) && viewModel.media.value.isEmpty()) {
            binding.composeEditField.error = getString(R.string.error_empty)
            enableButtons(true, viewModel.editing)
        } else if (characterCount <= maximumTootCharacters) {
            lifecycleScope.launch {
                try {
                    viewModel.sendStatus(contentText, spoilerText, activeAccount.id)
                    deleteDraftAndFinish()
                } catch (e: kotlinx.coroutines.CancellationException) {
                    // Preserve structured cancellation — re-throw so the
                    // Activity destroy / send-cancel path tears down the
                    // coroutine cleanly instead of running UI work after.
                    throw e
                } catch (e: IllegalStateException) {
                    // sendStatus check()s for media attached on the
                    // quote-retweet path; the message describes the
                    // specific cause so surface it instead of the
                    // generic error toast.
                    android.util.Log.w("ComposeActivity", "sendStatus failed", e)
                    displayTransientMessage(R.string.error_compose_quote_with_media)
                    enableButtons(true, viewModel.editing)
                } catch (e: Exception) {
                    // sendStatus throws on the quote-retweet path when the
                    // backend call fails. Keep the user's typed text on
                    // screen and re-enable the button so they can retry
                    // instead of losing the draft to a silent fire-and-
                    // forget failure.
                    android.util.Log.w("ComposeActivity", "sendStatus failed", e)
                    displayTransientMessage(R.string.error_generic)
                    enableButtons(true, viewModel.editing)
                }
            }
        } else {
            binding.composeEditField.error = getString(R.string.error_compose_character_limit)
            enableButtons(true, viewModel.editing)
        }
    }

    private fun initiateCameraApp() {
        addMediaBehavior.state = BottomSheetBehavior.STATE_HIDDEN

        val photoFile: File = try {
            createNewImageFile(this)
        } catch (ex: IOException) {
            displayTransientMessage(R.string.error_media_upload_opening)
            return
        }

        // Continue only if the File was successfully created
        photoUploadUri = FileProvider.getUriForFile(
            this,
            BuildConfig.APPLICATION_ID + ".fileprovider",
            photoFile
        ).also { uri -> takePictureLauncher.launch(uri) }
    }

    private fun enableButton(button: ImageButton, clickable: Boolean, colorActive: Boolean) {
        button.isEnabled = clickable
        setDrawableTint(
            this,
            button.drawable,
            if (colorActive) {
                android.R.attr.textColorTertiary
            } else {
                R.attr.textColorDisabled
            }
        )
    }

    private fun editImageInQueue(item: QueuedMedia) {
        // If input image is lossless, output image should be lossless.
        // Currently the only supported lossless format is png.
        val mimeType: String? = contentResolver.getType(item.uri)
        val isPng: Boolean = mimeType != null && mimeType.endsWith("/png")
        val tempFile = createNewImageFile(this, if (isPng) ".png" else ".jpg")

        // "Authority" must be the same as the android:authorities string in AndroidManifest.xml
        val uriNew = FileProvider.getUriForFile(
            this,
            BuildConfig.APPLICATION_ID + ".fileprovider",
            tempFile
        )

        viewModel.cropImageItemOld = item

        editImage.launch(
            EditImageOptions(
                input = item.uri,
                outputUri = uriNew,
                outputCompressFormat = if (isPng) Bitmap.CompressFormat.PNG else Bitmap.CompressFormat.JPEG
            )
        )
    }

    private fun removeMediaFromQueue(item: QueuedMedia) {
        viewModel.removeMediaFromQueue(item)
    }

    private fun showContentWarning(show: Boolean) {
        TransitionManager.beginDelayedTransition(
            binding.composeContentWarningBar.parent as ViewGroup
        )
        @AttrRes val color = if (show) {
            binding.composeContentWarningBar.show()
            binding.composeContentWarningField.setSelection(
                binding.composeContentWarningField.text.length
            )
            binding.composeContentWarningField.requestFocus()
            binding.composeContentWarningButton.setImageResource(R.drawable.ic_feedback_24dp_filled)
            appcompatR.attr.colorPrimary
        } else {
            binding.composeContentWarningBar.hide()
            binding.composeEditField.requestFocus()
            binding.composeContentWarningButton.setImageResource(R.drawable.ic_feedback_24dp)
            android.R.attr.textColorTertiary
        }
        binding.composeContentWarningButton.drawable.setTint(
            MaterialColors.getColor(
                binding.composeHideMediaButton,
                color
            )
        )
    }

    override fun onOptionsItemSelected(item: MenuItem): Boolean {
        if (item.itemId == android.R.id.home) {
            handleCloseButton()
            return true
        }

        return super.onOptionsItemSelected(item)
    }

    override fun onKeyDown(keyCode: Int, event: KeyEvent): Boolean {
        if (event.action == KeyEvent.ACTION_DOWN) {
            if (event.isCtrlPressed) {
                if (keyCode == KeyEvent.KEYCODE_ENTER) {
                    // send tweet by pressing CTRL + ENTER
                    this.onSendClicked()
                    return true
                }
            }

            if (keyCode == KeyEvent.KEYCODE_BACK) {
                onBackPressedDispatcher.onBackPressed()
                return true
            }
        }
        return super.onKeyDown(keyCode, event)
    }

    private fun handleCloseButton() {
        val contentText = binding.composeEditField.text.toString()
        val contentWarning = binding.composeContentWarningField.text.toString()
        when (viewModel.closeConfirmation.value) {
            ConfirmationKind.NONE -> {
                viewModel.stopUploads()
                finish()
            }
            ConfirmationKind.SAVE_OR_DISCARD ->
                getSaveAsDraftOrDiscardDialog(contentText, contentWarning).show()
            ConfirmationKind.UPDATE_OR_DISCARD ->
                getUpdateDraftOrDiscardDialog(contentText, contentWarning).show()
            ConfirmationKind.CONTINUE_EDITING_OR_DISCARD_CHANGES ->
                getContinueEditingOrDiscardDialog().show()
            ConfirmationKind.CONTINUE_EDITING_OR_DISCARD_DRAFT ->
                getDeleteEmptyDraftOrContinueEditing().show()
        }
    }

    /**
     * User is editing a new post, and can either save the changes as a draft or discard them.
     */
    private fun getSaveAsDraftOrDiscardDialog(
        contentText: String,
        contentWarning: String
    ): MaterialAlertDialogBuilder {
        val warning = if (viewModel.media.value.isNotEmpty()) {
            R.string.compose_save_draft_loses_media
        } else {
            R.string.compose_save_draft
        }

        return MaterialAlertDialogBuilder(this)
            .setMessage(warning)
            .setPositiveButton(R.string.action_save) { _, _ ->
                viewModel.stopUploads()
                saveDraftAndFinish(contentText, contentWarning)
            }
            .setNegativeButton(R.string.action_delete) { _, _ ->
                viewModel.stopUploads()
                deleteDraftAndFinish()
            }
    }

    /**
     * User is editing an existing draft, and can either update the draft with the new changes or
     * discard them.
     */
    private fun getUpdateDraftOrDiscardDialog(
        contentText: String,
        contentWarning: String
    ): MaterialAlertDialogBuilder {
        val warning = if (viewModel.media.value.isNotEmpty()) {
            R.string.compose_save_draft_loses_media
        } else {
            R.string.compose_save_draft
        }

        return MaterialAlertDialogBuilder(this)
            .setMessage(warning)
            .setPositiveButton(R.string.action_save) { _, _ ->
                viewModel.stopUploads()
                saveDraftAndFinish(contentText, contentWarning)
            }
            .setNegativeButton(R.string.action_discard) { _, _ ->
                viewModel.stopUploads()
                finish()
            }
    }

    /**
     * User is editing a post (scheduled, or posted), and can either go back to editing, or
     * discard the changes.
     */
    private fun getContinueEditingOrDiscardDialog(): MaterialAlertDialogBuilder {
        return MaterialAlertDialogBuilder(this)
            .setMessage(R.string.compose_unsaved_changes)
            .setPositiveButton(R.string.action_continue_edit) { _, _ ->
                // Do nothing, dialog will dismiss, user can continue editing
            }
            .setNegativeButton(R.string.action_discard) { _, _ ->
                viewModel.stopUploads()
                finish()
            }
    }

    /**
     * User is editing an existing draft and making it empty.
     * The user can either delete the empty draft or go back to editing.
     */
    private fun getDeleteEmptyDraftOrContinueEditing(): MaterialAlertDialogBuilder {
        return MaterialAlertDialogBuilder(this)
            .setMessage(R.string.compose_delete_draft)
            .setPositiveButton(R.string.action_delete) { _, _ ->
                viewModel.deleteDraft()
                viewModel.stopUploads()
                finish()
            }
            .setNegativeButton(R.string.action_continue_edit) { _, _ ->
                // Do nothing, dialog will dismiss, user can continue editing
            }
    }

    private fun deleteDraftAndFinish() {
        viewModel.deleteDraft()
        finish()
    }

    private fun saveDraftAndFinish(contentText: String, contentWarning: String) {
        lifecycleScope.launch {
            viewModel.saveDraft(contentText, contentWarning)
            finish()
        }
    }

    override fun search(token: String): List<ComposeAutoCompleteAdapter.AutocompleteResult> {
        return viewModel.searchAutocompleteSuggestions(token)
    }

    override fun onUpdateDescription(localId: Int, description: String) {
        viewModel.updateDescription(localId, description)
    }

    /**
     * Tweet' kind. This particularly affects how the status is handled if the user
     * backs out of the edit.
     */
    enum class ComposeKind {
        /** Tweet is new */
        NEW,

        /** Editing a posted status */
        EDIT_POSTED,

        /** Editing a status started as an existing draft */
        EDIT_DRAFT,

        /** Editing an an existing scheduled status */
        EDIT_SCHEDULED
    }

    @Parcelize
    data class ComposeOptions(
        val draftId: Int? = null,
        val content: String? = null,
        val mediaUrls: List<String>? = null,
        val mediaDescriptions: List<String>? = null,
        val mentionedUsernames: Set<String>? = null,
        val inReplyToId: String? = null,
        val replyVisibility: Tweet.Visibility? = null,
        val visibility: Tweet.Visibility? = null,
        val contentWarning: String? = null,
        val replyingStatusAuthor: String? = null,
        val replyingTweetContent: String? = null,
        val mediaAttachments: List<Attachment>? = null,
        val sensitive: Boolean? = null,
        val modifiedInitialState: Boolean? = null,
        val language: String? = null,
        val statusId: String? = null,
        val kind: ComposeKind? = null,
        // Quote retweet: when set the composed tweet is sent via the
        // PUBLIC_POST_RETWEET wire with the typed text used as the
        // comment instead of a fresh PRIVATE_POST_TWEET. quotedUserId
        // is the author of the source tweet (needed for the wire DTO).
        val quotedTweetId: String? = null,
        val quotedUserId: String? = null
    ) : Parcelable

    companion object {
        private const val TAG = "ComposeActivity" // logging tag

        internal const val COMPOSE_OPTIONS_EXTRA = "COMPOSE_OPTIONS"
        private const val PHOTO_UPLOAD_URI_KEY = "PHOTO_UPLOAD_URI"

        /**
         * @param options ComposeOptions to configure the ComposeActivity
         * @return an Intent to start the ComposeActivity
         */
        @JvmStatic
        fun newIntent(context: Context, options: ComposeOptions): Intent {
            return Intent(context, ComposeActivity::class.java).apply {
                putExtra(COMPOSE_OPTIONS_EXTRA, options)
            }
        }

        fun canHandleMimeType(mimeType: String?): Boolean {
            return mimeType != null &&
                (mimeType.startsWith("image/") || mimeType.startsWith("video/") || mimeType.startsWith("audio/") || mimeType == "text/plain")
        }

        /**
         * Calculate the effective status length.
         *
         * Some text is counted differently:
         *
         * In the status body:
         *
         * - URLs always count for [urlLength] characters irrespective of their actual length
         *   (https://docs.joinmastodon.org/user/posting/#links)
         * - Mentions ("@user@some.instance") only count the "@user" part
         *   (https://docs.joinmastodon.org/user/posting/#mentions)
         * - Hashtags are always treated as their actual length, including the "#"
         *   (https://docs.joinmastodon.org/user/posting/#hashtags)
         *
         * Content warning text is always treated as its full length, URLs and other entities
         * are not treated differently.
         *
         * @param body status body text
         * @param contentWarning optional content warning text
         * @param urlLength the number of characters attributed to URLs
         * @return the effective status length
         */
        @JvmStatic
        fun statusLength(body: Spanned, contentWarning: Spanned?, urlLength: Int): Int {
            var length = body.toString().perceivedCharacterLength() - body.getSpans(0, body.length, URLSpan::class.java)
                .fold(0) { acc, span ->
                    // Accumulate a count of characters to be *ignored* in the final length
                    acc + when (span) {
                        is MentionSpan -> {
                            // Ignore everything from the second "@" (if present)
                            span.url.length - (
                                span.url.indexOf("@", 1).takeIf { it >= 0 }
                                    ?: span.url.length
                                )
                        }
                        else -> {
                            // Expected to be negative if the URL length < maxUrlLength
                            span.url.perceivedCharacterLength() - urlLength
                        }
                    }
                }

            // Content warning text is treated as is, URLs or mentions there are not special
            contentWarning?.let { length += it.toString().perceivedCharacterLength() }
            return length
        }

        // String.length would count emojis as multiple characters but Warpnet counts them as 1, so we need this workaround
        private fun String.perceivedCharacterLength(): Int {
            val breakIterator = BreakIterator.getCharacterInstance()
            breakIterator.setText(this)
            var count = 0
            while (breakIterator.next() != BreakIterator.DONE) {
                count++
            }
            return count
        }
    }
}
