#include "udt.h"
#include "udt_c_connector.h"

const UDTSOCKET INVALID_SOCK = UDT::INVALID_SOCK;
const int ERROR = UDT::ERROR;

uint16_t _htons(uint16_t hostshort) {
   return htons(hostshort);
}

UDT_API int udt_startup() {
   return UDT::startup();
}

UDT_API int udt_cleanup() {
   return UDT::cleanup();
}
UDT_API UDTSOCKET udt_socket(int af, int type, int protocol) {
   return UDT::socket(af, type, protocol);
}

UDT_API int udt_bind(UDTSOCKET u, const struct sockaddr* name, int namelen) {
   return UDT::bind(u, name, namelen);
}

UDT_API int udt_bind2(UDTSOCKET u, UDPSOCKET udpsock) {
   return UDT::bind2(u, udpsock);
}

UDT_API int udt_listen(UDTSOCKET u, int backlog) {
   return UDT::listen(u, backlog);
}

UDT_API UDTSOCKET udt_accept(UDTSOCKET u, struct sockaddr* addr, int* addrlen) {
   return UDT::accept(u, addr, addrlen);
}

UDT_API int udt_connect(UDTSOCKET u, const struct sockaddr* name, int namelen) {
   return UDT::connect(u, name, namelen);
}

UDT_API int udt_close(UDTSOCKET u) {
   return UDT::close(u);
}

UDT_API int udt_getpeername(UDTSOCKET u, struct sockaddr* name, int* namelen) {
   return UDT::getpeername(u, name, namelen);
}

UDT_API int udt_getsockname(UDTSOCKET u, struct sockaddr* name, int* namelen) {
   return UDT::getsockname(u, name, namelen);
}

UDT_API int udt_getsockopt(UDTSOCKET u, int level, UDT::SOCKOPT optname, void* optval, int* optlen) {
   return UDT::getsockopt(u, level, optname, optval, optlen);
}

UDT_API int udt_setsockopt(UDTSOCKET u, int level, UDT::SOCKOPT optname, const void* optval, int optlen) {
   return UDT::setsockopt(u, level, optname, optval, optlen);
}
UDT_API int udt_send(UDTSOCKET u, const char* buf, int len, int flags) {
   return UDT::send(u, buf, len, flags);
}

UDT_API int udt_recv(UDTSOCKET u, char* buf, int len, int flags) {
   return UDT::recv(u, buf, len, flags);
}

UDT_API int udt_sendmsg(UDTSOCKET u, const char* buf, int len, int ttl, int inorder) {
   return UDT::sendmsg(u, buf, len, ttl, inorder);
}

UDT_API int udt_recvmsg(UDTSOCKET u, char* buf, int len) {
   return UDT::recvmsg(u, buf, len);
}

UDT_API int64_t udt_sendfile2(UDTSOCKET u, const char* path, int64_t* offset, int64_t size, int block) {
   return UDT::sendfile2(u, path, offset, size, block);
}

UDT_API int64_t udt_recvfile2(UDTSOCKET u, const char* path, int64_t* offset, int64_t size, int block) {
   return UDT::recvfile2(u, path, offset, size, block);
}

UDT_API int udt_epoll_create() {
   return UDT::epoll_create();
}

UDT_API int udt_epoll_add_usock(int eid, UDTSOCKET u, const int* events) {
   return UDT::epoll_add_usock(eid, u, events);
}

UDT_API int udt_epoll_add_ssock(int eid, SYSSOCKET s, const int* events) {
   return UDT::epoll_add_ssock(eid, s, events);
}

UDT_API int udt_epoll_remove_usock(int eid, UDTSOCKET u) {
   return UDT::epoll_remove_usock(eid, u);
}

UDT_API int udt_epoll_remove_ssock(int eid, SYSSOCKET s) {
   return UDT::epoll_remove_ssock(eid, s);
}

UDT_API int udt_epoll_wait2(int eid, UDTSOCKET* readfds, int* rnum, UDTSOCKET* writefds, int* wnum, int64_t msTimeOut,
   SYSSOCKET* lrfds, int* lrnum, SYSSOCKET* lwfds, int* lwnum) {
   return UDT::epoll_wait2(eid, readfds, rnum, writefds, wnum, msTimeOut, lrfds, lrnum, lwfds, lwnum);
}

UDT_API int udt_epoll_release(int eid) {
   return UDT::epoll_release(eid);
}

UDT_API int udt_getlasterror_code() {
   return UDT::getlasterror_code();
}

UDT_API const char* udt_getlasterror_desc() {
   return UDT::getlasterror_desc();
}

UDT_API int udt_perfmon(UDTSOCKET u, TRACEINFO* perf, int clear) {
   return UDT::perfmon(u, perf, clear);
}

UDT_API UDTSTATUS udt_getsockstate(UDTSOCKET u) {
   return UDT::getsockstate(u);
}

