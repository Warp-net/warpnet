package site.warpnet.warpdroid.util

import android.content.Context
import com.bumptech.glide.Glide
import com.bumptech.glide.Registry
import com.bumptech.glide.annotation.GlideModule
import com.bumptech.glide.module.AppGlideModule
import java.nio.ByteBuffer

@GlideModule
class GlideModule : AppGlideModule() {
    /**
     * Hook the synthetic "warpnet://avatar/…" URLs emitted by
     * [site.warpnet.warpdroid.warpnet.WarpnetMapper] into Glide's loader
     * chain. `prepend` (not `append`) so our loader sees the URL before
     * Glide's built-in String→URI resolver tries to dial it as a content://
     * path and fails with ENOENT.
     */
    override fun registerComponents(context: Context, glide: Glide, registry: Registry) {
        registry.prepend(
            String::class.java,
            ByteBuffer::class.java,
            WarpnetAvatarLoader.Factory.forContext(context),
        )
    }
}
