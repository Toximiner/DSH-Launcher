#!/usr/bin/python3
"""Подставной сервер уведомлений (org.freedesktop.Notifications) на сессионной
шине D-Bus — как у GNOME: уведомление лежит в списке, пока его не закроют.
  notifyd.py   (DBUS_SESSION_BUS_ADDRESS — шина)
Журнал — /tmp/notifyd.log: «notify id=N …», «close id=N»; открытые сейчас —
/tmp/notifyd-open.json (список id): столько и насчитала бы док-панель.
"""
import json
import time
import warnings

from gi.repository import Gio, GLib

warnings.filterwarnings('ignore', category=DeprecationWarning)  # register_object: новый API есть не везде

LOG = '/tmp/notifyd.log'
OPEN = '/tmp/notifyd-open.json'
XML = '''<node><interface name="org.freedesktop.Notifications">
  <method name="Notify">
    <arg type="s" direction="in"/><arg type="u" direction="in"/><arg type="s" direction="in"/>
    <arg type="s" direction="in"/><arg type="s" direction="in"/><arg type="as" direction="in"/>
    <arg type="a{sv}" direction="in"/><arg type="i" direction="in"/><arg type="u" direction="out"/>
  </method>
  <method name="CloseNotification"><arg type="u" direction="in"/></method>
  <method name="GetCapabilities"><arg type="as" direction="out"/></method>
  <method name="GetServerInformation">
    <arg type="s" direction="out"/><arg type="s" direction="out"/><arg type="s" direction="out"/><arg type="s" direction="out"/>
  </method>
  <signal name="NotificationClosed"><arg type="u"/><arg type="u"/></signal>
  <signal name="ActionInvoked"><arg type="u"/><arg type="s"/></signal>
</interface></node>'''

opened = []
next_id = [1]


def log(msg):
    with open(LOG, 'a') as f:
        f.write(f'{int(time.time() * 1000)} {msg}\n')


def save():
    with open(OPEN, 'w') as f:
        json.dump(opened, f)


def on_call(conn, _sender, path, iface, method, params, invocation):
    if method == 'Notify':
        app, replaces, _icon, summary, body, _actions, _hints, _timeout = params.unpack()
        nid = replaces if replaces in opened else next_id[0]
        if nid == next_id[0]:
            next_id[0] += 1
            opened.append(nid)
        save()
        log(f'notify id={nid} app={app} summary={summary} body={body}')
        invocation.return_value(GLib.Variant('(u)', (nid,)))
    elif method == 'CloseNotification':
        (nid,) = params.unpack()
        if nid in opened:
            opened.remove(nid)
            save()
        log(f'close id={nid}')
        invocation.return_value(None)
        conn.emit_signal(None, path, iface, 'NotificationClosed', GLib.Variant('(uu)', (nid, 3)))
    elif method == 'GetCapabilities':
        invocation.return_value(GLib.Variant('(as)', (['body', 'actions'],)))
    else:
        invocation.return_value(GLib.Variant('(ssss)', ('notifyd', 'test', '1', '1.2')))


def on_bus(conn, _name):
    node = Gio.DBusNodeInfo.new_for_xml(XML)
    conn.register_object('/org/freedesktop/Notifications', node.interfaces[0], on_call, None, None)


def on_name(_conn, _name):
    save()
    log('listening')


Gio.bus_own_name(Gio.BusType.SESSION, 'org.freedesktop.Notifications', Gio.BusNameOwnerFlags.NONE,
                 on_bus, on_name, None)
GLib.MainLoop().run()
