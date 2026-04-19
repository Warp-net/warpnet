// import {getClient} from "@/lib/client";
//
// const PUBLIC_GET_TWEET = "/public/get/tweet/0.0.0"
// const PUBLIC_GET_TWEET_STATS   = "/public/get/tweetstats/0.0.0"
// const PRIVATE_GET_TIMELINE = "/private/get/timeline/0.0.0"
// const PUBLIC_GET_TWEETS = "/public/get/tweets/0.0.0"
// const PUBLIC_POST_UNLIKE = "/public/post/unlike/0.0.0"
// const PRIVATE_POST_TWEET = "/private/post/tweet/0.0.0"
// const PUBLIC_POST_REPLY = "/public/post/reply/0.0.0"
// const PUBLIC_GET_FOLLOWINGS = "/public/get/followings/0.0.0"
// const PUBLIC_GET_REPLY = "/public/get/reply/0.0.0"
// const PRIVATE_GET_STATS = "/private/get/admin/stats/0.0.0"
// const PRIVATE_POST_RESET       = "/private/post/reset/0.0.0"
// const PUBLIC_DELETE_REPLY = "/public/delete/reply/0.0.0"
// const PRIVATE_DELETE_TWEET = "/private/delete/tweet/0.0.0"
// const PRIVATE_POST_USER = "/private/post/user/0.0.0"
// const PUBLIC_POST_UNFOLLOW = "/public/post/unfollow/0.0.0"
// const PUBLIC_GET_USER = "/public/get/user/0.0.0"
// const PUBLIC_GET_USERS = "/public/get/users/0.0.0"
// const PUBLIC_GET_WHOTOFOLLOW = "/public/get/whotofollow/0.0.0"
// const PUBLIC_GET_REPLIES = "/public/get/replies/0.0.0"
// const PUBLIC_GET_FOLLOWERS = "/public/get/followers/0.0.0"
// const PUBLIC_POST_FOLLOW = "/public/post/follow/0.0.0"
// const PUBLIC_POST_LIKE = "/public/post/like/0.0.0"
// const PUBLIC_POST_RETWEET = "/public/post/retweet/0.0.0"
// const PUBLIC_POST_UNRETWEET = "/public/post/unretweet/0.0.0"
// const PUBLIC_POST_CHAT = "/public/post/chat/0.0.0"
// const PRIVATE_DELETE_CHAT = "/private/delete/chat/0.0.0"
// const PRIVATE_GET_CHATS = "/private/get/chats/0.0.0"
// const PRIVATE_GET_CHAT = "/private/get/chat/0.0.0"
// const PRIVATE_GET_MESSAGES = "/private/get/messages/0.0.0"
// const PUBLIC_POST_MESSAGE = "/public/post/message/0.0.0"
// const PRIVATE_GET_MESSAGE = "/private/get/message/0.0.0"
// const PUBLIC_DELETE_MESSAGE = "/public/delete/message/0.0.0"
// const PRIVATE_POST_UPLOAD_IMAGE = "/private/post/image/0.0.0"
// const PUBLIC_GET_IMAGE = "/public/get/image/0.0.0"
//
// const PRIVATE_POST_LOGIN = "/private/post/login/0.0.0"
// const PRIVATE_POST_LOGOUT = "/private/post/logout/0.0.0"
//
// const client = getClient();
//
// export default {
//
//   async loginUser(form) {
//     let id = await client.sendMessage({
//       path: PRIVATE_POST_LOGIN,
//       body: {
//         username: form.username,
//         password: form.password,
//       }
//     });
//
//     let resp = await client.getResponse(id)
//     if (!resp) {
//       throw new Error("authentication failed")
//     }
//     if (resp && resp.code && resp.code === 500) {
//       throw new Error(resp.message);
//     }
//     return resp;
//   },
//
//   async logoutUser(ownerNodeId) {
//     await client.sendMessage({
//       path: PRIVATE_POST_LOGOUT,
//       node_id: ownerNodeId,
//       body: {}
//     })
//     client.close();
//   },
//
//   async getProfile({ownerNodeId, userId}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_GET_USER,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: userId,
//       },
//     })
//     return await client.getResponse(id)
//   },
//
//   async uploadImage({ownerNodeId, imgFile}) {
//     let id = await client.sendMessage({
//       path: PRIVATE_POST_UPLOAD_IMAGE,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         file: imgFile,
//       },
//     })
//     const resp =  await client.getResponse(id)
//     if (resp && resp.code === 500) {
//       console.error(resp.message)
//       return null;
//     }
//     return resp.key
//   },
//
//   async getImage({ownerNodeId, userId, key}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_GET_IMAGE,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: userId,
//         key: key,
//       }
//     })
//     const resp = await client.getResponse(id)
//     if (resp && resp.code === 500) {
//       console.error(resp.message)
//       return null;
//     }
//     return resp.file
//   },
//
//   // do not cache!
//   async getNodeInfo(ownerNodeId) {
//     let id = await client.sendMessage({
//       path: PRIVATE_GET_STATS,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {},
//     })
//     return await client.getResponse(id)
//   },
//
//   async resetConsensus(ownerNodeId) {
//     let id = await client.sendMessage({
//       path: PRIVATE_POST_RESET,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {},
//     })
//     return await client.getResponse(id)
//   },
//
//   async getUsers({ownerNodeId, userId, limit, cursor}) {
//     console.log("client: get users request:", ownerNodeId, userId, limit, cursor)
//     let id = await client.sendMessage({
//       path: PUBLIC_GET_USERS,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         limit: limit,
//         cursor: cursor,
//         user_id: userId,
//       },
//     });
//
//     return await client.getResponse(id)
//   },
//
//   async getWhoToFollow({ownerNodeId, userId, limit, cursor}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_GET_WHOTOFOLLOW,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         limit: limit,
//         cursor: cursor,
//         user_id: userId,
//       },
//     });
//
//     return await client.getResponse(id)
//   },
//
//   async getMyTimeline({ownerNodeId, ownerUserId, limit, cursor}) {
//     let id = await client.sendMessage({
//       path: PRIVATE_GET_TIMELINE,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         limit: limit,
//         cursor: cursor,
//         user_id: ownerUserId,
//       },
//     });
//
//     return await client.getResponse(id)
//   },
//   async createTweet({ownerNodeId, ownerUserId, ownerUsername, text, imageKey}) {
//     let id = await client.sendMessage({
//       path: PRIVATE_POST_TWEET,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: ownerUserId,
//         username: ownerUsername,
//         text: text,
//         image_key: imageKey,
//         created_at: new Date().toISOString(),
//       },
//     })
//
//     return await client.getResponse(id)
//   },
//   async deleteTweet({ownerNodeId, userId, tweetId}) {
//     let id = await client.sendMessage({
//       path: PRIVATE_DELETE_TWEET,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: userId,
//         tweet_id: tweetId,
//       },
//     })
//
//     return await client.getResponse(id)
//   },
//   async getTweet({ownerNodeId, userId, tweetId}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_GET_TWEET,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: userId,
//         tweet_id: tweetId,
//       },
//     })
//
//     return await client.getResponse(id)
//   },
//
//   async getTweetStats({ownerNodeId, userId, tweetId}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_GET_TWEET_STATS,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: userId,
//         tweet_id: tweetId,
//       },
//     })
//
//     return await client.getResponse(id)
//   },
//
//   async getTweets({ownerNodeId, userId, limit, cursor}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_GET_TWEETS,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         limit: limit,
//         cursor: cursor,
//         user_id: userId,
//       },
//     })
//
//     return await client.getResponse(id)
//   },
//
//   async followUser({ownerNodeId, ownerUserId, profileId}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_POST_FOLLOW,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         following: profileId,
//         follower: ownerUserId,
//       },
//     })
//     return await client.getResponse(id); // count
//   },
//
//   async unfollowUser({ownerNodeId, ownerUserId, profileId}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_POST_UNFOLLOW,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         following: profileId,
//         follower: ownerUserId,
//       },
//     })
//     return await client.getResponse(id);
//   },
//
//   async getFollowers({ownerNodeId, userId, limit, cursor} ) {
//     let id = await client.sendMessage({
//       path: PUBLIC_GET_FOLLOWERS,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: userId,
//         cursor: cursor,
//         limit: limit,
//       },
//     })
//
//     return await client.getResponse(id)
//   },
//
//   async getFollowing({ownerNodeId, userId, limit, cursor}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_GET_FOLLOWINGS,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: userId,
//         cursor: cursor,
//         limit: limit,
//       },
//     })
//
//
//     return await client.getResponse(id)
//   },
//
//   async likeTweet({ownerNodeId,ownerUserId, userId, tweetId}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_POST_LIKE,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: userId,
//         tweet_id: tweetId,
//         owner_id: ownerUserId,
//       },
//     })
//
//     const resp = await client.getResponse(id);
//     return resp.count;
//   },
//
//   async unlikeTweet({ownerNodeId, ownerUserId,userId, tweetId}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_POST_UNLIKE,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: userId,
//         tweet_id: tweetId,
//         owner_id: ownerUserId,
//       },
//     })
//
//     const resp = await client.getResponse(id);
//     return resp.count;
//   },
//
//   async editMyProfile({ownerNodeId, newProfile}) {
//     let id = await client.sendMessage({
//       path: PRIVATE_POST_USER,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         bio: newProfile.bio,
//         avatar: newProfile.avatar,
//         avatar_key: newProfile.avatar_key,
//         username: newProfile.username,
//         background_image: newProfile.background_image,
//         background_image_key: newProfile.background_image_key,
//         website: newProfile.website,
//         birthdate: newProfile.birthdate,
//       },
//     })
//
//     return await client.getResponse(id);
//   },
//
//   async replyTweet({ownerNodeId, ownerUserId, rootId, parentId, parentUserId, username, text} ) {
//     let id = await client.sendMessage({
//       path: PUBLIC_POST_REPLY,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         root_id: rootId,
//         parent_id: parentId,
//         user_id: ownerUserId,
//         username: username,
//         parent_user_id: parentUserId,
//         text: text,
//         created_at: new Date().toISOString(),
//       },
//     })
//
//     return await client.getResponse(id)
//   },
//
//   async getTweetReply({ownerNodeId, rootId, replyId}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_GET_REPLY,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         root_id: rootId,
//         reply_id: replyId,
//       },
//     })
//
//     return await client.getResponse(id)
//   },
//
//   async deleteTweetReply({ownerNodeId, userId, rootId, replyId}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_DELETE_REPLY,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: userId,
//         root_id: rootId,
//         reply_id: replyId,
//       },
//     })
//
//     return await client.getResponse(id)
//   },
//
//   async getReplies({ownerNodeId, rootId, parentId, limit, cursor}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_GET_REPLIES,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         root_id: rootId,
//         parent_id: parentId,
//         limit: limit,
//         cursor: cursor,
//       },
//     })
//
//     return await client.getResponse(id)
//   },
//
//   async retweetTweet({ownerNodeId, ownerUserId, tweetId, userId, username, text}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_POST_RETWEET,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         id: tweetId,
//         user_id: userId,
//         username: username,
//         text: text,
//         retweeted_by: ownerUserId,
//         created_at: new Date().toISOString(),
//       },
//     })
//
//     return await client.getResponse(id) // retweets count
//   },
//   async unretweetTweet({ownerNodeId, ownerUserId, tweetId}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_POST_UNRETWEET,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         retweeter_id: ownerUserId,
//         tweet_id: tweetId,
//       },
//     })
//
//     return await client.getResponse(id); // retweets count
//   },
//
//   async createChat({ownerNodeId, ownerUserId, otherUserId}) {
//     let id = await client.sendMessage({
//       path: PUBLIC_POST_CHAT,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         owner_id: ownerUserId,
//         other_user_id: otherUserId,
//       },
//     })
//
//     return await client.getResponse(id);
//   },
//
//   async getChat({ownerNodeId, chatId}) {
//     let id = await client.sendMessage({
//       path: PRIVATE_GET_CHAT,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         chat_id: chatId,
//       },
//     })
//
//     return await client.getResponse(id);
//   },
//
//
//   async getChats({ownerNodeId, ownerUserId, limit, cursor}) {
//     let id = await client.sendMessage({
//       path: PRIVATE_GET_CHATS,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         user_id: ownerUserId,
//         limit: limit,
//         cursor: cursor,
//       },
//     })
//
//     return await client.getResponse(id);
//   },
//
//   async deleteChat({ownerNodeId, chatId}) {
//     let id = await client.sendMessage({
//       path: PRIVATE_DELETE_CHAT,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         chat_id: chatId,
//       },
//     })
//
//     return  await client.getResponse(id);
//   },
//
//   async getDirectMessages({ownerNodeId, ownerUserId, chatId, limit, cursor} ) {
//     let id = await client.sendMessage({
//       path: PRIVATE_GET_MESSAGES,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         owner_id: ownerUserId,
//         chat_id: chatId,
//         limit: limit,
//         cursor: cursor,
//       },
//     })
//
//     return await client.getResponse(id);
//   },
//
//   async sendDirectMessage({ownerNodeId, ownerUserId, receiverId, chatId, text} ) {
//     let id = await client.sendMessage({
//       path: PUBLIC_POST_MESSAGE,
//       node_id: ownerNodeId,
//       timestamp: new Date().toISOString(),
//       body: {
//         sender_id: ownerUserId,
//         receiver_id: receiverId,
//         chat_id: chatId,
//         text: text,
//       },
//     })
//
//     return await client.getResponse(id);
//   },
// };
//   // setActiveChat({chat, otherUser}) {
//   //
//   // },
//   // getActiveChat({ _ }) {
//   //   console.log("active chat get")
//   //   if (!convo) return undefined;
//   //   try {
//   //     return JSON.parse(convo);
//   //   } catch (e) {
//   //     console.error("Failed to parse activeChat:", e);
//   //     return undefined;
//   //   }
//   // },
//   // resetMessages({ _ }) { // ?
//   //   commit("WARPNET_MESSAGES_RESET");
//   // },
//
//   // NOTIFICATIONS are not implemented yet
//   // async subscribeNotifications({ commit, getters, dispatch }) {
//     // if (!getters.profile.id || getters.subscription) return;
//     // const isFromActiveChat = (
//     //   userId,
//     //   notification,
//     //   activeChat
//     // ) => {
//     //   const chatId =
//     //     userId < notification.otherUserId
//     //       ? `${userId}_${notification.otherUserId}`
//     //       : `${notification.otherUserId}_${userId}`;
//     //   return activeChat && activeChat.id === chatId;
//     // };
//     //
//     // const userId = getters.profile.id;
//     // const subscription = getOnNotifiedSubscription(userId).subscribe({
//     //   next: async ({ value }) => {
//     //     const notification = value.data.onNotified;
//     //     if (notification.type === "DMed") {
//     //       await dispatch("loadChats", 20);
//     //       // only load messages if they are from the active chat
//     //       if (
//     //         isFromActiveChat(userId, notification, getters.chat)
//     //       ) {
//     //         await dispatch("getDirectMessages", {
//     //           limit: 10,
//     //           message: notification.message,
//     //           otherUserId: notification.otherUserId,
//     //         });
//     //       }
//     //       commit("WARPNET_MESSAGES_NEW", notification);
//     //     } else {
//     //       await dispatch("getMyTimeline", 20); //cheeky update to see latest data
//     //       commit("WARPNET_NOTIFICATIONS_NEW", notification);
//     //     }
//     //   },
//     // });
//     // commit("WARPNET_NOTIFICATIONS_SUBSCRIBE", subscription);
//   // },
//
//   // TODO think how to implement p2p search. Now it's a backlog
//   // async loadSearch({ commit }, { query, mode, limit }) {
//     // const searchResults = await search(query, mode, limit);
//     // commit("WARPNET_SEARCH", searchResults);
//   // },
//   // async loadMoreSearch({ commit, getters }, { query, mode, limit }) {
//     // if (!getters.cursorSearch) return;
//     // const searchResults = await search(
//     //   query,
//     //   mode,
//     //   limit,
//     //   getters.cursorSearch
//     // );
//     // commit("WARPNET_LOADMORE_SEARCH", searchResults);
//   // },
//   // resetSearch({ commit }) {
//   //   const searchResults = {
//   //     results: [],
//   //     cursor: undefined,
//   //   };
//   //   commit("WARPNET_SEARCH", searchResults);
//   // },
//   // async loadSearchHashTag({ commit }, { query, mode, limit }) {
//     // const q = query || " "; // mandatory field
//     // const searchResults = await getHashTag(q, mode, limit);
//     // commit("WARPNET_SEARCH_HASHTAG", searchResults);
//   // },
//   // async loadMoreSearchHashTag({ commit, getters }, { query, mode, limit }) {
//     // if (!getters.cursorSearch) return;
//     // const q = query || " "; // mandatory field
//     // const searchResults = await getHashTag(
//     //   q,
//     //   mode,
//     //   limit,
//     //   getters.cursorSearch
//     // );
//     // commit("WARPNET_LOADMORE_SEARCH_HASHTAG", searchResults);
//   // },
//   // resetSearchHashTag({ commit }) {
//   //   const searchResults = {
//   //     results: [],
//   //     cursor: undefined,
//   //   };
//   //   commit("WARPNET_SEARCH_HASHTAG", searchResults);
//   // },
//
// // };
