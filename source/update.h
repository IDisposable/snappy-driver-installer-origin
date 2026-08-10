/*
This file is part of Snappy Driver Installer Origin.

Snappy Driver Installer Origin is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by the Free Software
Foundation, either version 3 of the License or (at your option) any later version.

Snappy Driver Installer Origin is distributed in the hope that it will be useful
but WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
FITNESS FOR A PARTICULAR PURPOSE.  See the GNU General Public License for more details.

You should have received a copy of the GNU General Public License along with
Snappy Driver Installer Origin.  If not, see <http://www.gnu.org/licenses/>.
*/

#ifndef UPDATE_H
#define UPDATE_H

#include <string>

#ifdef USE_TORRENT
#include <libtorrent/torrent_handle.hpp>
#include <libtorrent/torrent_status.hpp>
#endif // USE_TORRENT

// Declarations
class Updater_t;
class Canvas;

// Global variables
extern Updater_t *Updater;

struct type_item {
    wchar_t ItemName[BUFSIZ];
    int SizeMB;
    int Percent;
    int VersionNew;
    int VersionCurrent;
    wchar_t ForThisPC[BUFSIZ];
    int DefaultSort;
    };

// Updater
class Updater_t
{
public:
    static int torrentport,outgoingport_min,outgoingport_max,downlimit,uplimit,connections;
    static int torrentalerts;
    static const std::wstring torrent_url;
    static const std::wstring torrent2_url;
    static const std::wstring torrent_save_path;
    static const std::wstring torrent2_save_path;
public:
    virtual ~Updater_t(){}

    virtual void ShowProgress(wchar_t *buf)=0;
    virtual void ShowPopup(Canvas &canvas)=0;

    virtual void checkUpdates()=0;
    virtual void pause()=0;

//    virtual bool isTorrentReady()=0;
    virtual bool IsPaused()=0;
    virtual bool IsUpdateCompleted()=0;
    virtual bool IsSeedingDrivers()=0;

    virtual int  Populate(bool reload)=0;
//    virtual void SetFilePriority(const wchar_t *name,int pri)=0;
    virtual void SetLimits()=0;
    virtual void OpenDialog(int automode=0)=0;
    virtual void StartSpecialShare()=0;
    virtual void StopSpecialShare()=0;
    virtual void StopTorrent()=0;
    virtual void SetActiveTorrent(const int torrent)=0;
//    virtual bool NextTorrent()=0;
    virtual void StartTorrent()=0;
    virtual void StartInstallDownload(std::vector<std::wstring> filenames)=0;
    virtual void EndInstallDownload()=0;

    #ifdef USE_TORRENT
    virtual std::wstring TorrentStateStr(libtorrent::torrent_status::state_t state)=0;
    virtual void WelcomeDownloadAll()=0;
    virtual void WelcomeDownloadNetwork()=0;
    virtual void WelcomeDownloadIndexes()=0;

    virtual int scriptInitUpdates(int torrentport)=0;
    virtual int scriptDownloadApp()=0;
    virtual int scriptDownloadIndexes()=0;
    virtual int scriptDownloadDrivers(std::wstring mode)=0;
    virtual int scriptDownloadEverything()=0;
    virtual int scriptDoDownload()=0;
    virtual int scriptInstall()=0;
    #endif // USE_TORRENT
};
Updater_t *CreateUpdater();

#endif

#ifdef USE_TORRENT
struct torrent_item {
    std::string FileName;
    std::string URL;
    std::string SavePath;
    libtorrent::torrent_handle handle;
    };
#endif // USE_TORRENT
