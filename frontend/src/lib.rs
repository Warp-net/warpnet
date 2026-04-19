use rust_embed::RustEmbed;
use std::borrow::Cow;

#[derive(RustEmbed)]
#[folder = "dist/"]
pub struct FrontendAssets;

pub fn get_file(path: &str) -> Option<Cow<'static, [u8]>> {
    FrontendAssets::get(path).map(|f| f.data)
}
