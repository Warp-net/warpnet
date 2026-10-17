public enum WarpnetLimits {
  public static let maxTweetBytes = 2048
  public static let minPollOptions = 2
  public static let maxPollOptions = 4
  public static let maxPollOptionChars = 25
  public static let maxImagesPerTweet = 4
  public static let maxVideoBytes = 36 * 1024 * 1024
  public static let maxReportReasonRunes = 256
  public static let acceptedVideoMimeTypes: Set<String> = [
    "video/mp4",
    "video/quicktime",
    "video/x-m4v",
  ]
}
