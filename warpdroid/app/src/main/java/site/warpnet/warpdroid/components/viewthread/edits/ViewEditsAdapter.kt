package site.warpnet.warpdroid.components.viewthread.edits

import android.content.Context
import android.graphics.Typeface.DEFAULT_BOLD
import android.graphics.drawable.Drawable
import android.text.Editable
import android.text.SpannableStringBuilder
import android.text.TextPaint
import android.text.style.CharacterStyle
import android.util.TypedValue
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import androidx.core.graphics.drawable.toDrawable
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.bumptech.glide.Glide
import com.google.android.material.color.MaterialColors
import site.warpnet.warpdroid.R
import site.warpnet.warpdroid.adapter.PollAdapter
import site.warpnet.warpdroid.adapter.PollAdapter.Companion.MULTIPLE
import site.warpnet.warpdroid.adapter.PollAdapter.Companion.SINGLE
import site.warpnet.warpdroid.databinding.ItemStatusEditBinding
import site.warpnet.warpdroid.entity.Attachment.Focus
import site.warpnet.warpdroid.entity.StatusEdit
import site.warpnet.warpdroid.interfaces.LinkListener
import site.warpnet.warpdroid.util.AbsoluteTimeFormatter
import site.warpnet.warpdroid.util.BindingHolder
import site.warpnet.warpdroid.util.BlurhashDrawable
import site.warpnet.warpdroid.util.WarpdroidTagHandler
import site.warpnet.warpdroid.util.aspectRatios
import site.warpnet.warpdroid.util.emojify
import site.warpnet.warpdroid.util.hide
import site.warpnet.warpdroid.util.parseAsWarpnetHtml
import site.warpnet.warpdroid.util.setClickableText
import site.warpnet.warpdroid.util.show
import site.warpnet.warpdroid.util.visible
import site.warpnet.warpdroid.viewdata.toViewData
import org.xml.sax.XMLReader

class ViewEditsAdapter(
    private val edits: List<StatusEdit>,
    private val animateEmojis: Boolean,
    private val useBlurhash: Boolean,
    private val listener: LinkListener
) : RecyclerView.Adapter<BindingHolder<ItemStatusEditBinding>>() {

    private val absoluteTimeFormatter = AbsoluteTimeFormatter()

    /** Size of large text in this theme, in px */
    private var largeTextSizePx: Float = 0f

    /** Size of medium text in this theme, in px */
    private var mediumTextSizePx: Float = 0f

    override fun onCreateViewHolder(
        parent: ViewGroup,
        viewType: Int
    ): BindingHolder<ItemStatusEditBinding> {
        val binding = ItemStatusEditBinding.inflate(
            LayoutInflater.from(parent.context),
            parent,
            false
        )

        binding.statusEditMediaPreview.clipToOutline = true

        val typedValue = TypedValue()
        val context = binding.root.context
        val displayMetrics = context.resources.displayMetrics
        context.theme.resolveAttribute(R.attr.status_text_large, typedValue, true)
        largeTextSizePx = typedValue.getDimension(displayMetrics)
        context.theme.resolveAttribute(R.attr.status_text_medium, typedValue, true)
        mediumTextSizePx = typedValue.getDimension(displayMetrics)

        return BindingHolder(binding)
    }

    override fun onBindViewHolder(holder: BindingHolder<ItemStatusEditBinding>, position: Int) {
        val edit = edits[position]

        val binding = holder.binding

        val context = binding.root.context

        val infoStringRes = if (position == edits.lastIndex) {
            R.string.status_created_info
        } else {
            R.string.status_edit_info
        }

        // Show the most recent version of the status using large text to make it clearer for
        // the user, and for similarity with thread view.
        val variableTextSize = if (position == edits.lastIndex) {
            mediumTextSizePx
        } else {
            largeTextSizePx
        }
        binding.statusEditContentWarningDescription.setTextSize(
            TypedValue.COMPLEX_UNIT_PX,
            variableTextSize
        )
        binding.statusEditContent.setTextSize(TypedValue.COMPLEX_UNIT_PX, variableTextSize)
        binding.statusEditMediaSensitivity.setTextSize(TypedValue.COMPLEX_UNIT_PX, variableTextSize)

        val timestamp = absoluteTimeFormatter.format(edit.createdAt, false)

        binding.statusEditInfo.text = context.getString(infoStringRes, timestamp)

        if (edit.spoilerText.isEmpty()) {
            binding.statusEditContentWarningDescription.hide()
            binding.statusEditContentWarningSeparator.hide()
        } else {
            binding.statusEditContentWarningDescription.show()
            binding.statusEditContentWarningSeparator.show()
            binding.statusEditContentWarningDescription.text = edit.spoilerText.emojify(
                edit.emojis,
                binding.statusEditContentWarningDescription,
                animateEmojis
            )
        }

        val emojifiedText = edit
            .content
            .parseAsWarpnetHtml(EditsTagHandler(context))
            .emojify(edit.emojis, binding.statusEditContent, animateEmojis)

        setClickableText(
            binding.statusEditContent,
            emojifiedText,
            emptyList(),
            emptyList(),
            listener,
        )

        if (edit.poll == null) {
            binding.statusEditPollOptions.hide()
            binding.statusEditPollDescription.hide()
        } else {
            binding.statusEditPollOptions.show()

            // not used for now since not reported by the api
            // https://github.com/mastodon/mastodon/issues/22571
            // binding.statusEditPollDescription.show()

            val pollAdapter = PollAdapter()
            binding.statusEditPollOptions.adapter = pollAdapter
            binding.statusEditPollOptions.layoutManager = LinearLayoutManager(context)

            pollAdapter.setup(
                options = edit.poll.options.map { it.toViewData(false) },
                voteCount = 0,
                votersCount = null,
                emojis = edit.emojis,
                mode = if (edit.poll.multiple) { // not reported by the api
                    MULTIPLE
                } else {
                    SINGLE
                },
                resultClickListener = null,
                animateEmojis = animateEmojis,
                enabled = false
            )
        }

        if (edit.mediaAttachments.isEmpty()) {
            binding.statusEditMediaPreview.hide()
            binding.statusEditMediaSensitivity.hide()
        } else {
            binding.statusEditMediaPreview.show()
            binding.statusEditMediaPreview.aspectRatios = edit.mediaAttachments.aspectRatios()

            binding.statusEditMediaPreview.forEachIndexed { index, imageView, descriptionIndicator ->

                val attachment = edit.mediaAttachments[index]
                val hasDescription = !attachment.description.isNullOrBlank()

                if (hasDescription) {
                    imageView.contentDescription = attachment.description
                } else {
                    imageView.contentDescription =
                        imageView.context.getString(R.string.action_view_media)
                }
                descriptionIndicator.visibility = if (hasDescription) View.VISIBLE else View.GONE

                val blurhash = attachment.blurhash

                val placeholder: Drawable = if (blurhash != null && useBlurhash) {
                    BlurhashDrawable(context, blurhash)
                } else {
                    MaterialColors.getColor(imageView, R.attr.colorBackgroundAccent).toDrawable()
                }

                if (attachment.previewUrl.isNullOrEmpty()) {
                    imageView.removeFocalPoint()
                    Glide.with(imageView)
                        .load(placeholder)
                        .centerInside()
                        .into(imageView)
                } else {
                    val focus: Focus? = attachment.meta?.focus

                    if (focus != null) {
                        imageView.setFocalPoint(focus)
                        Glide.with(imageView.context)
                            .load(attachment.previewUrl)
                            .placeholder(placeholder)
                            .centerInside()
                            .addListener(imageView)
                            .into(imageView)
                    } else {
                        imageView.removeFocalPoint()
                        Glide.with(imageView)
                            .load(attachment.previewUrl)
                            .placeholder(placeholder)
                            .centerInside()
                            .into(imageView)
                    }
                }
            }
            binding.statusEditMediaSensitivity.visible(edit.sensitive)
        }
    }

    override fun getItemCount() = edits.size

    companion object {
        private const val VIEW_TYPE_EDITS_NEWEST = 0
        private const val VIEW_TYPE_EDITS = 1
    }
}

/**
 * Handle XML tags created by [ViewEditsViewModel] and create custom spans to display inserted or
 * deleted text.
 */
class EditsTagHandler(val context: Context) : WarpdroidTagHandler() {
    /** Class to mark the start of a span of deleted text */
    class Del

    /** Class to mark the start of a span of inserted text */
    class Ins

    override fun handleTag(opening: Boolean, tag: String, output: Editable, xmlReader: XMLReader) {
        when (tag) {
            DELETED_TEXT_EL -> {
                if (opening) {
                    start(output as SpannableStringBuilder, Del())
                } else {
                    end(
                        output as SpannableStringBuilder,
                        Del::class.java,
                        DeletedTextSpan(context)
                    )
                }
            }
            INSERTED_TEXT_EL -> {
                if (opening) {
                    start(output as SpannableStringBuilder, Ins())
                } else {
                    end(
                        output as SpannableStringBuilder,
                        Ins::class.java,
                        InsertedTextSpan(context)
                    )
                }
            }
            else -> super.handleTag(opening, tag, output, xmlReader)
        }
    }

    /** Span that signifies deleted text */
    class DeletedTextSpan(context: Context) : CharacterStyle() {
        private var bgColor: Int

        init {
            bgColor = context.getColor(R.color.view_edits_background_delete)
        }

        override fun updateDrawState(tp: TextPaint) {
            tp.bgColor = bgColor
            tp.isStrikeThruText = true
        }
    }

    /** Span that signifies inserted text */
    class InsertedTextSpan(context: Context) : CharacterStyle() {
        private var bgColor: Int

        init {
            bgColor = context.getColor(R.color.view_edits_background_insert)
        }

        override fun updateDrawState(tp: TextPaint) {
            tp.bgColor = bgColor
            tp.typeface = DEFAULT_BOLD
        }
    }

    companion object {
        /** XML element to represent text that has been deleted */
        // Can't be an element that Android's HTML parser recognises, otherwise the tagHandler
        // won't be called for it.
        const val DELETED_TEXT_EL = "warpdroid-del"

        /** XML element to represent text that has been inserted */
        // Can't be an element that Android's HTML parser recognises, otherwise the tagHandler
        // won't be called for it.
        const val INSERTED_TEXT_EL = "warpdroid-ins"
    }
}
