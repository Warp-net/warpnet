/*

 Warpnet - Decentralized Social Network
 Copyright (C) 2025 Vadim Filin, https://github.com/Warp-net,
 <github.com.mecdy@passmail.net>

 This program is free software: you can redistribute it and/or modify
 it under the terms of the GNU Affero General Public License as published by
 the Free Software Foundation, either version 3 of the License, or
 (at your option) any later version.

 This program is distributed in the hope that it will be useful,
 but WITHOUT ANY WARRANTY; without even the implied warranty of
 MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 GNU Affero General Public License for more details.

 You should have received a copy of the GNU Affero General Public License
 along with this program.  If not, see <https://www.gnu.org/licenses/>.

 WarpNet is provided "as is" without warranty of any kind, either expressed or implied.
 Use at your own risk. The maintainers shall not be liable for any damages or data loss
 resulting from the use or misuse of this software.
*/

import {warpnetService} from "@/service/service";

// Chats with people outside the circle that the owner neither started nor
// answered wait in a list of message requests. The host lists its chats as
// listedChats and may have an active chat.
export default {
  data() {
    return {
      unfamiliarUsers: {},
      showRequests: null,
    };
  },
  computed: {
    requestChats() {
      return this.listedChats.filter((chat) => this.unfamiliarUsers[chat.other_user_id] === true);
    },
    isShowingRequests() {
      if (this.showRequests !== null) return this.showRequests;
      return this.unfamiliarUsers[this.active?.other_user_id] === true;
    },
    visibleChats() {
      const isRequest = (chat) => this.unfamiliarUsers[chat.other_user_id] === true;
      return this.listedChats.filter((chat) => isRequest(chat) === this.isShowingRequests);
    },
  },
  methods: {
    checkFamiliarity(userId) {
      if (!userId || userId in this.unfamiliarUsers) return;
      this.unfamiliarUsers[userId] = false;
      warpnetService.isUnfamiliar(userId)
        .then((isUnfamiliar) => { this.unfamiliarUsers[userId] = isUnfamiliar; })
        .catch(() => {});
    },
    acceptChatWith(userId) {
      if (!userId) return;
      warpnetService.acceptChat(userId);
      this.unfamiliarUsers[userId] = false;
    },
  },
};
