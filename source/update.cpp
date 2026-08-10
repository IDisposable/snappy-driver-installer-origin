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


#define BOOST_ASIO_STANDALONE
#ifdef USE_TORRENT
#include "libtorrent/torrent_info.hpp"
#include "libtorrent/session.hpp"
#include "libtorrent/add_torrent_params.hpp"
#include "libtorrent/torrent_handle.hpp"
#include "libtorrent/alert_types.hpp"
#include "libtorrent/session.hpp"
#include "libtorrent/settings_pack.hpp"
#endif // USE_TORRENT

#include <fstream>
#include <shlwapi.h>

#include "com_header.h"
#include <stdio.h>
#include <string.h>

#include "common.h"
#include "logging.h"
#include "settings.h"
#include "system.h"
#include "matcher.h"
#include "update.h"
#include "manager.h"
#include "theme.h"
#include "gui.h"
#include "draw.h"
#include "install.h"
//#include <direct.h>     // _wgetcwd

#include <iostream>
#include <thread>
#include <chrono>

#include <windows.h>
#include <shobjidl.h>

#include "main.h"

#define SMOOTHING_FACTOR 0.005l

#ifdef USE_TORRENT
using namespace libtorrent;
#endif // USE_TORRENT

// UpdateDialog
class UpdateDialog_t
{
    static const int cxn[];
    static WNDPROC wpOrigButtonProc;
    static int bMouseInWindow;
    static HWND hUpdate;
    bool UpdateInProgress;
    int SelectedCount;
    int64_t totalsize;
    int64_t totalavail;
    int AutoMode=0;
    COLORREF PromptColor;
    HFONT hDialogFont,hPromptFont,hButtonFont;

private:
    int  getnewver(const char *ptr);
    int  getcurver(const char *ptr);
    void calctotalsize();
    void calcavailablespace();
    void InitTexts();
    void UpdateTexts();
    bool PerformAutoMode();
    void ResizeForm(int width, int height);
    static LRESULT CALLBACK NewButtonProc(HWND hWnd,UINT uMsg,WPARAM wParam,LPARAM lParam);
    static INT_PTR CALLBACK UpdateProcedure(HWND hwnd,UINT Message,WPARAM wParam,LPARAM lParam);

public:
    int setPriorities();
    int setCheckboxes();
    void SetFonts();
    void UpdatePromptMessage();
    void UpdateTorrentStats();
    void UpdateTorrentAlert(const std::string msg);
    int  Populate(bool reload);
    void Populate2();
    void UpdateProgress();
    void OpenDialog(int automode=0);
    long LocalRevision;
    long TorrentRevision;
    void SetControls();
    void SetAllDriverCheckboxes();
    void SetCheckbox(int id,int checked);
};

// Updater
class UpdaterImp:public Updater_t
{
    static Event *ThreadManager_event;
    static ThreadAbs *thandle_download;

    static int ThreadManager_exitflag;
    static bool finishedupdating;
    static bool InstallDownloadRunning;

private:
    void LoadTorrents();
    #ifdef USE_TORRENT
    int CreateTorrentFromFile(std::string FileName, std::string SavePath, libtorrent::torrent_handle &handle, bool SeedMode);
    int GetTorrentNumFromAlert(const libtorrent::torrent_alert* alert);
    #endif // USE_TORRENT
    void StartTorrentSession();
    void FlushTorrentCache(const int torrent_num);
    void StartSpecialShare();
    void StopSpecialShare();
    void StartTorrent();
    void StartInstallDownload(std::vector<std::wstring> filenames);
    void EndInstallDownload();
    int SetFilePriorities();
    void RemoveOldDriverpacks(const wchar_t *ptr);
    void RemoveDriverPack(const wchar_t *ptr);
    void RemoveRedundantDriverPacks();
    int MoveNewFiles();
    static unsigned int __stdcall thread_download(void *arg);

public:
    UpdaterImp();
    ~UpdaterImp();

    void ShowProgress(wchar_t *buf);
    void ShowPopup(Canvas &canvas);

    void checkUpdates();
    void pause();

    void SetActiveTorrent(const int torrent);
    void StopTorrent();
    void ProcessFinishedTorrent();

    bool IsPaused();
    bool IsUpdateCompleted();
    bool IsSeedingDrivers();

    int  Populate(bool reload);
    void Populate2();
    void SetLimits();
    void OpenDialog(int automode=0);

    #ifdef USE_TORRENT
    std::wstring TorrentStateStr(libtorrent::torrent_status::state_t state);
    void WelcomeDownloadAll();
    void WelcomeDownloadNetwork();
    void WelcomeDownloadIndexes();

    int scriptInitUpdates(int _torrentport);
    int scriptDownloadApp();
    int scriptDownloadIndexes();
    int scriptDownloadDrivers(std::wstring mode);
    int scriptDownloadEverything();
    void scriptRemaining();
    int scriptDoDownload();
    int scriptInstall();
    #endif // USE_TORRENT
};
Updater_t *CreateUpdater(){return new UpdaterImp;}

//{ Global variables
#ifdef USE_TORRENT
libtorrent::session *hSession=nullptr;
#endif // USE_TORRENT
int activetorrent=0;
UpdateDialog_t UpdateDialog;
Updater_t *Updater;
int ListViewSortColumn=666;
bool ListViewSortAsc=TRUE;
std::int64_t TorrentStartTime=0;
int AverageSpeed=0;
bool ListViewUpdating=false;
bool IsInitializing=false;
bool IsMovingFiles=false;
bool IsResetting=false;
bool IsFlushing=false;
bool IsStartingShareMode=false;
bool IsEndingShareMode=false;
extern bool CRITICAL_SECTION_ACTIVE;

enum THREAD_STATUS
{
    THREAD_STATUS_WAITING,     // set in UpdaterImp constructor
    THREAD_STATUS_WORKING,     //
    THREAD_STATUS_ABORT,       // set in destructor
};

// UpdateDialog (static)
// listview column widths
const int UpdateDialog_t::cxn[]={260,60,44,80,80,70};
HWND UpdateDialog_t::hUpdate=nullptr;
WNDPROC UpdateDialog_t::wpOrigButtonProc;
int UpdateDialog_t::bMouseInWindow=0;

#ifdef USE_TORRENT
// the internal directories in the torrent are appended to the save path
std::vector<torrent_item> Torrents =
{
    {
        ".\\torrent\\SDIO_Update.torrent",
        "http://www.snappy-driver-installer.org/downloads/SDIO_Update.torrent",
        ".\\update",
        { }
    },
    {
        ".\\torrent\\Drivers.torrent",
        "http://www.snappy-driver-installer.org/downloads/Drivers.torrent",
        ".\\update\\SDIO_Update",
        { }
    }
};
#endif // USE_TORRENT

// Updater (static)

int Updater_t::torrentport=50171;
int Updater_t::outgoingport_min=0;
int Updater_t::outgoingport_max=0;
int Updater_t::downlimit=0;
int Updater_t::uplimit=0;
int Updater_t::connections=0;
int Updater_t::torrentalerts=0;
int UpdaterImp::ThreadManager_exitflag;
bool UpdaterImp::finishedupdating;
bool UpdaterImp::InstallDownloadRunning;
Event *UpdaterImp::ThreadManager_event=nullptr;
ThreadAbs *UpdaterImp::thandle_download=nullptr;
//}



//{ ListView
class ListView_t
{
public:
    HWND hListg;

    void init(HWND hwnd)
    {
        hListg=GetDlgItem(hwnd,ID_UPD_LIST);
        SendMessage(hListg,LVM_SETEXTENDEDLISTVIEWSTYLE,0,LVS_EX_CHECKBOXES|LVS_EX_FULLROWSELECT|LVS_EX_DOUBLEBUFFER);
    }
    void close()
    {
        // free ItemData
        LVITEM item;
        type_item *ItemData;
        for(int i=0;i<GetItemCount();i++)
        {
            item.iItem=i;
            item.mask=LVIF_PARAM;
            GetItem(&item);
            ItemData=(type_item*)item.lParam;
            delete ItemData;
        }
        hListg=nullptr;
    }
    bool IsVisible(){ return hListg!=nullptr; }
    static LPARAM CALLBACK CompareFunc(LPARAM lParam1,LPARAM lParam2,LPARAM lParamSort)
    {
        // the first two parameters are the ItemData structures to be compared
        // use the third parameter to control which column is used and
        // whether it's ascending or descending
        // -1 -2 -3 sort descending,  1 2 3 sort ascending

        bool isAsc = (lParamSort > 0);
        int column = abs(lParamSort)-1;

        type_item *ItemData1=(type_item*)lParam1;
        type_item *ItemData2=(type_item*)lParam2;
        int nRet=0;

        // default
        if(column==666)
        {
            if(ItemData1->DefaultSort>ItemData2->DefaultSort)nRet=1;
            else if(ItemData1->DefaultSort<ItemData2->DefaultSort)nRet=-1;
        }
        // item name
        else if(column==0)
            nRet=wcscmp(ItemData1->ItemName,ItemData2->ItemName);
        // size
        else if(column==1)
        {
            if(ItemData1->SizeMB>ItemData2->SizeMB)nRet=1;
            else if(ItemData2->SizeMB>ItemData1->SizeMB)nRet=-1;
        }
        // percent
        else if(column==2)
        {
            if(ItemData1->Percent>ItemData2->Percent)nRet=1;
            else if(ItemData2->Percent>ItemData1->Percent)nRet=-1;
        }
        // new ver
        else if(column==3)
        {
            if(ItemData1->VersionNew>ItemData2->VersionNew)nRet=1;
            else if(ItemData2->VersionNew>ItemData1->VersionNew)nRet=-1;
        }
        // current ver
        else if(column==4)
        {
            if(ItemData1->VersionCurrent>ItemData2->VersionCurrent)nRet=1;
            else if(ItemData2->VersionCurrent>ItemData1->VersionCurrent)nRet=-1;
        }
        // for this pc
        if(column==5)
        {
            if(ItemData1->ForThisPC>ItemData2->ForThisPC)nRet=1;
            else if(ItemData1->ForThisPC<ItemData2->ForThisPC)nRet=-1;
        }
        if(!isAsc)nRet=nRet*-1;
        return nRet;
    }
    int GetItemCount()
    {
        return ListView_GetItemCount(hListg);
    }
    int GetCheckState(int i)
    {
        return ListView_GetCheckState(hListg,i);
    }
    void SetCheckState(int i,int val)
    {
        ListView_SetCheckState(hListg,i,val);
    }
    void SetItemState(int i,UINT32 state,UINT32 mask)
    {
        ListView_SetItemState(hListg,i,state,mask);
    }
    void GetItemText(int i,int sub,wchar_t *buf,int sz)
    {
        ListView_GetItemText(hListg,i,sub,buf,sz);
    }
    int InsertItem(const LVITEM *lvI)
    {
        return ListView_InsertItem(hListg,lvI);
    }
    void GetItem(LVITEM *item)
    {
        SendMessage(hListg,LVM_GETITEM,0,(LPARAM)item);
    }
    void InsertColumn(int i,const LVCOLUMN *lvc)
    {
        SendMessage(hListg,LVM_INSERTCOLUMN,i,reinterpret_cast<LPARAM>(lvc));
    }
    void SetColumn(int i,const LVCOLUMN *lvc)
    {
        SendMessage(hListg,LVM_SETCOLUMN,i,reinterpret_cast<LPARAM>(lvc));
    }
    void SetItemTextUpdate(int iItem,int iSubItem,const wchar_t *str)
    {
        wchar_t buf[BUFLEN];

        *buf=0;
        ListView_GetItemText(hListg,iItem,iSubItem,buf,BUFLEN);
        if(wcscmp(str,buf)!=0)
            ListView_SetItemText(hListg,iItem,iSubItem,const_cast<wchar_t *>(str));
    }
};
ListView_t ListView;

//}


/*
 *
 *                         THE UPDATE DIALOG
 *
*/

//{ UpdateDialog
int UpdateDialog_t::getnewver(const char *s)
{
    // looking for the digits in the file name
    while(*s)
    {
        if(*s=='_'&&s[1]>='0'&&s[1]<='9')
            return atoi(s+1);

        s++;
    }
    return 0;
}

int UpdateDialog_t::getcurver(const char *ptr)
{
    WStringShort bffw;

    // looking for digits in the file name

    bffw.sprintf(L"%S",ptr);
    wchar_t *s=bffw.GetV();
    while(*s)
    {
        if(*s=='_'&&s[1]>='0'&&s[1]<='9')
        {
            *s=0;
            s=const_cast<wchar_t *>(manager_g->matcher->finddrp(bffw.Get()));
            if(!s)return 0;
            while(*s)
            {
                if(*s==L'_'&&s[1]>=L'0'&&s[1]<=L'9')
                    return _wtoi_my(s+1);
                s++;
            }
            return 0;
        }
        s++;
    }
    return 0;
}

void UpdateDialog_t::calctotalsize()
{
    totalsize=0;
    SelectedCount=0;
    for(int i=0;i<ListView.GetItemCount();i++)
        if(ListView.GetCheckState(i))
        {
            wchar_t buf[BUFLEN];
            ListView.GetItemText(i,1,buf,32);
            totalsize+=_wtoi64_my(buf)*1024*1024;
            SelectedCount++;
        }
}

void UpdateDialog_t::calcavailablespace()
{
    // calculate available space on the download drive in bytes
    // for now this is the same as the exe path
    ULARGE_INTEGER lpFreeBytesAvailable;
    totalavail=0;
    if(GetDiskFreeSpaceEx(nullptr,
                          &lpFreeBytesAvailable,
                          nullptr,
                          nullptr))totalavail=lpFreeBytesAvailable.QuadPart;
}

void UpdateDialog_t::InitTexts()
{
    // initialize the dialog texts
    wchar_t spec[BUFLEN];
    wcscpy(spec,Settings.drp_dir);
    wcscat(spec,L"\\DP_*.7z");

    if(!hUpdate)return;

    SetWindowText(hUpdate,STR(STR_UPD_TITLE));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_SELECTION),STR(STR_UPD_SELECTION));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_CHECKALL),STR(STR_UPD_BTN_ALL));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_UNCHECKALL),STR(STR_UPD_BTN_NONE));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_CHECKNETWORK),STR(STR_UPD_BTN_NETWORK));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_CHECKTHISPC),STR(STR_UPD_BTN_THISPC));

    SetWindowText(GetDlgItem(hUpdate,ID_UPD_SELECTED),STR(STR_UPD_SELECTED));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_TOTALSIZE),STR(STR_UPD_TOTALSIZE));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_TOTALAVAIL),STR(STR_UPD_TOTALAVAIL));

    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_STATS),STR(STR_UPD_STREAMSTATS));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_STATE),STR(STR_UPD_STREAM_STATE));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_COMPLETE),STR(STR_UPD_STREAM_COMPLETE));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_TIME),STR(STR_UPD_STREAM_TIME));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_SESSDL),STR(STR_UPD_STREAM_SESSDL));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_SESSUL),STR(STR_UPD_STREAM_SESSUL));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_SEEDS),STR(STR_UPD_STREAM_SEEDS));
    if(Updater->torrentalerts)
        SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_ALERT),STR(STR_UPD_STREAM_ALERT));
    else
        SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_ALERT),L"");


    SetWindowText(GetDlgItem(hUpdate,ID_UPD_OPTIONS),STR(STR_UPD_OPTIONS));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_ONLYUPDATES),STR(STR_UPD_ONLYUPDATES));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_KEEPSEEDING),STR(STR_UPD_KEEPSEEDING));

    SetWindowText(GetDlgItem(hUpdate,ID_UPD_SPECIALMODE),STR(STR_UPD_SPECIALMODE));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_TXT_SHARE),STR(STR_UPD_TXT_SHARE));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_BTN_SHARE),STR(STR_UPD_BTN_SHARE));

    SetWindowText(GetDlgItem(hUpdate,ID_UPD_BTN_UPDATE),STR(STR_UPD_BTN_UPDATE));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_BTN_STOP),STR(STR_UPD_BTN_STOP));
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_BTN_CLOSE),STR(STR_UPD_BTN_CLOSE));


    // Column headers
    LVCOLUMN lvc;
    lvc.mask=LVCF_TEXT;
    for(int i=0;i<6;i++)
    {
        lvc.pszText=const_cast<wchar_t *>(STR(STR_UPD_COL_NAME+i));
        ListView.SetColumn(i,&lvc);
    }
}

void UpdateDialog_t::UpdateTexts()
{
    // update the various dialog texts

    if(!hUpdate)return;

    int TotalItems;
    wchar_t buf[BUFLEN];
    std::wstring ws1,ws2;

    // total size
    ws1=STR(STR_UPD_TOTALSIZE);
    ws2=BytesToStr(totalsize);
    wsprintf(buf,ws1.c_str(),ws2.c_str());
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_TOTALSIZE),buf);

    // available space
    ws1=STR(STR_UPD_TOTALAVAIL);
    ws2=BytesToStr(totalavail);
    wsprintf(buf,ws1.c_str(),ws2.c_str());
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_TOTALAVAIL),buf);

    // selected items
    TotalItems=ListView.GetItemCount();
    ws1=STR(STR_UPD_SELECTED);
    wsprintf(buf,ws1.c_str(),SelectedCount,TotalItems);
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_SELECTED),buf);
}

bool UpdateDialog_t::PerformAutoMode()
{
    // this is used by the Welcome dialog to automate a few tasks
    wchar_t buf[32];

    std::wstring indexes=STR(STR_UPD_INDEXES);
    int cnt=0;
    switch(AutoMode)
    {
        case 1:
            // download everything
            for(int i=0;i<ListView.GetItemCount();i++)
                ListView.SetCheckState(i,1);
            Updater->StartTorrent();
            return true;
        case 2:
            // download network drivers only
            for(int i=0;i<ListView.GetItemCount();i++)
            {
                *buf=0;
                ListView.GetItemText(i,0,buf,32);
                bool chk=StrStrIW(buf,L"_Net_")||StrStrIW(buf,L"_LAN_")||StrStrIW(buf,L"_WLAN-WiFi_")||StrStrIW(buf,L"_WWAN-4G_")||StrStrIW(buf,L"Indexes");
                ListView.SetCheckState(i,chk);
                if(chk)cnt++;
            }
            if(cnt)
                Updater->StartTorrent();
            return true;
        case 3:
            // download indexes only
            for(int i=0;i<ListView.GetItemCount();i++)
            {
                *buf=0;
                ListView.GetItemText(i,0,buf,32);
                bool chk=StrStrIW(buf,indexes.c_str());
                ListView.SetCheckState(i,chk);
                if(chk)cnt++;
            }
            if(cnt)
            {
                Updater->StartTorrent();
            }
            return true;
        case 4:
            // download this pc only
            for(int i=0;i<ListView.GetItemCount();i++)
            {
                *buf=0;
                ListView.GetItemText(i,5,buf,32);
                ListView.SetCheckState(i,StrStrIW(buf,STR(STR_UPD_YES))?1:0);
            }
            Updater->StartTorrent();
            return true;
        default:
            return false;

    }
}

void UpdateDialog_t::ResizeForm(int width, int height)
{
    // adjust all the controls position and size
    // as the form size changes

    HWND hwnd;
    RECT rc;

    // the original dialog converted to pixels
    rc.left=0; rc.top=0; rc.right=440; rc.bottom=410;
    MapDialogRect(hUpdate,&rc);
    LONG origWidth=rc.right-rc.left; LONG origHeight=rc.bottom-rc.top;

    // CENTRE

    // list view
    hwnd=GetDlgItem(hUpdate,ID_UPD_LIST);
    // these are the number from the resources.rc file
    rc.left=5;rc.top=5;rc.right=rc.left+429;rc.bottom=rc.top+200;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top;
        LONG w=rc.right+width-origWidth;      LONG h=rc.bottom-rc.top+height-origHeight;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    //

    // progress bar
    hwnd=GetDlgItem(hUpdate,ID_UPD_PROGRESSBAR);
    rc.left=5;rc.top=208;rc.right=rc.left+429;rc.bottom=rc.top+21;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right+width-origWidth;      LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // user prompt
    hwnd=GetDlgItem(hUpdate,ID_UPD_PROMPT);
    rc.left=15;rc.top=212;rc.right=rc.left+419;rc.bottom=rc.top+14;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right+width-origWidth;      LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // LEFT SIDE

    // selection box
    hwnd=GetDlgItem(hUpdate,ID_UPD_SELECTION);
    rc.left=5;rc.top=234;rc.right=rc.left+208;rc.bottom=rc.top+80;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // check all
    hwnd=GetDlgItem(hUpdate,ID_UPD_CHECKALL);
    rc.left=10;rc.top=244;rc.right=rc.left+94;rc.bottom=rc.top+14;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // uncheck all
    hwnd=GetDlgItem(hUpdate,ID_UPD_UNCHECKALL);
    rc.left=10;rc.top=260;rc.right=rc.left+94;rc.bottom=rc.top+14;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // this pc
    hwnd=GetDlgItem(hUpdate,ID_UPD_CHECKTHISPC);
    rc.left=111;rc.top=244;rc.right=rc.left+94;rc.bottom=rc.top+14;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // network
    hwnd=GetDlgItem(hUpdate,ID_UPD_CHECKNETWORK);
    rc.left=111;rc.top=260;rc.right=rc.left+94;rc.bottom=rc.top+14;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // selected
    hwnd=GetDlgItem(hUpdate,ID_UPD_SELECTED);
    rc.left=9;rc.top=278;rc.right=rc.left+160;rc.bottom=rc.top+12;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // download size
    hwnd=GetDlgItem(hUpdate,ID_UPD_TOTALSIZE);
    rc.left=9;rc.top=289;rc.right=rc.left+160;rc.bottom=rc.top+12;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // available space
    hwnd=GetDlgItem(hUpdate,ID_UPD_TOTALAVAIL);
    rc.left=9;rc.top=300;rc.right=rc.left+160;rc.bottom=rc.top+12;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // LEFT SIDE

    // stream stats box
    hwnd=GetDlgItem(hUpdate,ID_UPD_STREAM_STATS);
    rc.left=5;rc.top=316;rc.right=rc.left+208;rc.bottom=rc.top+91;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left+width-origWidth;   LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // stream state
    hwnd=GetDlgItem(hUpdate,ID_UPD_STREAM_STATE);
    rc.left=9;rc.top=326;rc.right=rc.left+196;rc.bottom=rc.top+11;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // stream wanted
    hwnd=GetDlgItem(hUpdate,ID_UPD_STREAM_COMPLETE);
    rc.left=9;rc.top=336;rc.right=rc.left+196;rc.bottom=rc.top+11;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // stream remaining
    hwnd=GetDlgItem(hUpdate,ID_UPD_STREAM_TIME);
    rc.left=9;rc.top=346;rc.right=rc.left+196;rc.bottom=rc.top+11;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // stream sessdl
    hwnd=GetDlgItem(hUpdate,ID_UPD_STREAM_SESSDL);
    rc.left=9;rc.top=356;rc.right=rc.left+196;rc.bottom=rc.top+11;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // stream sessul
    hwnd=GetDlgItem(hUpdate,ID_UPD_STREAM_SESSUL);
    rc.left=9;rc.top=366;rc.right=rc.left+196;rc.bottom=rc.top+11;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // stream seeds
    hwnd=GetDlgItem(hUpdate,ID_UPD_STREAM_SEEDS);
    rc.left=9;rc.top=376;rc.right=rc.left+196;rc.bottom=rc.top+11;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // stream alert
    hwnd=GetDlgItem(hUpdate,ID_UPD_STREAM_ALERT);
    rc.left=9;rc.top=386;rc.right=rc.left+196;rc.bottom=rc.top+19;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left;                       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left+width-origWidth;   LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // RIGHT SIDE

    // options box
    hwnd=GetDlgItem(hUpdate,ID_UPD_OPTIONS);
    rc.left=225;rc.top=232;rc.right=rc.left+209;rc.bottom=rc.top+58;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left+width-origWidth;       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // only updates
    hwnd=GetDlgItem(hUpdate,ID_UPD_ONLYUPDATES);
    rc.left=230;rc.top=242;rc.right=rc.left+200;rc.bottom=rc.top+24;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left+width-origWidth;       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // keep seeding
    hwnd=GetDlgItem(hUpdate,ID_UPD_KEEPSEEDING);
    rc.left=230;rc.top=262;rc.right=rc.left+200;rc.bottom=rc.top+24;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left+width-origWidth;       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    //

    //  share mode box
    hwnd=GetDlgItem(hUpdate,ID_UPD_SPECIALMODE);
    // these are the numbers in the resources.rc description of the control
    rc.left=225;rc.top=298;rc.right=rc.left+209;rc.bottom=rc.top+44;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left+width-origWidth;       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    //  mode text
    hwnd=GetDlgItem(hUpdate,ID_UPD_TXT_SHARE);
    // these are the numbers in the resources.rc description of the control
    rc.left=230;rc.top=309;rc.right=rc.left+152;rc.bottom=rc.top+28;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left+width-origWidth;       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    //  mode button
    hwnd=GetDlgItem(hUpdate,ID_UPD_BTN_SHARE);
    // these are the numbers in the resources.rc description of the control
    rc.left=385;rc.top=312;rc.right=rc.left+45;rc.bottom=rc.top+20;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left+width-origWidth;       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }


    //

    // start button
    hwnd=GetDlgItem(hUpdate,ID_UPD_BTN_UPDATE);
    rc.left=235;rc.top=378;rc.right=rc.left+55;rc.bottom=rc.top+28;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left+width-origWidth;       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // stop button
    hwnd=GetDlgItem(hUpdate,ID_UPD_BTN_STOP);
    rc.left=307;rc.top=378;rc.right=rc.left+55;rc.bottom=rc.top+28;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left+width-origWidth;       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }

    // close button
    hwnd=GetDlgItem(hUpdate,ID_UPD_BTN_CLOSE);
    rc.left=379;rc.top=378;rc.right=rc.left+55;rc.bottom=rc.top+28;
    if(MapDialogRect(hUpdate,&rc))
    {
        LONG x=rc.left+width-origWidth;       LONG y=rc.top+height-origHeight;
        LONG w=rc.right-rc.left;              LONG h=rc.bottom-rc.top;
        MoveWindow(hwnd, x, y, w, h, TRUE);
    }
}

int UpdateDialog_t::setCheckboxes()
{
    // called during WM_INITDIALOG
    // if the torrent is active this will recheck the items
    // that are being downloaded
    // if the torrent is not active, no change

    #ifdef USE_TORRENT

    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return 0;
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();

    int ret=0;
    std::string s;

    // The app and indexes
    int baseChecked=0,indexesChecked=0;
    for(int i=0;i<info->num_files();i++)
        if(CurrentTorrent.file_priority(i)==2)
        {
            s=fs.file_path(i);
            if(StrStrIA(s.c_str(),"indexes\\"))
                indexesChecked=1;
            else
                baseChecked=1;
        }

    // Driverpacks
    for(int i=0;i<ListView.GetItemCount();i++)
    {
        LVITEM item;
        item.mask=LVIF_PARAM;
        item.iItem=i;
        ListView.GetItem(&item);
        int val=0;
        type_item *ItemData=(type_item*)item.lParam;

        if(ItemData->DefaultSort==-2)val=baseChecked;
        if(ItemData->DefaultSort==-1)val=indexesChecked;
        if(ItemData->DefaultSort>=0)val=CurrentTorrent.file_priority((int)ItemData->DefaultSort);

        ListView.SetCheckState(i,val);
        if(val)
            ret++;
    }
    return ret;
    #else
    return 0;
    #endif // USE_TORRENT
}

int UpdateDialog_t::setPriorities()
{
    #ifdef USE_TORRENT

    // set the torrent priorities based on the list items checkboxes
    int count=0;
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return 0;
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();
    libtorrent::torrent_status status=CurrentTorrent.status(status_flags_t::all());

    // Clear priorities for driverpacks
    for(int i=0;i<info->num_files();i++)
    {
        std::string fp=fs.file_path(i);
        if(StrStrIA(fp.c_str(),"drivers\\"))
            CurrentTorrent.file_priority(i,0);
    }

    // Set priorities for driverpacks
    int base_pri=0,indexes_pri=0;
    for(int i=0;i<ListView.GetItemCount();i++)
    {
        // get each list view item
        LVITEM item;
        item.mask=LVIF_PARAM;
        item.iItem=i;
        ListView.GetItem(&item);
        // get the item check state
        int val=ListView.GetCheckState(i);
        if(val>0)
            count++;

        type_item *ItemData=(type_item*)item.lParam;
        // app priority will be 2 if checked
        if(ItemData->DefaultSort==-2)base_pri=val?2:0;
        // index priority will be 2 if checked
        if(ItemData->DefaultSort==-1)indexes_pri=val?2:0;
        // driver priority will be 1 if checked
        if(ItemData->DefaultSort>= 0)CurrentTorrent.file_priority(static_cast<int>(ItemData->DefaultSort),val);
    }

    // i set the driver priorites just above so only need to set
    // priorities for any torrent file that's not a driver
    for(int i=0;i<info->num_files();i++)
    {
        std::string fp=fs.file_path(i);
        if(!StrStrIA(fp.c_str(),"drivers\\"))
            CurrentTorrent.file_priority(i,StrStrIA(fp.c_str(),"indexes\\")?indexes_pri:base_pri);
    }
    return count;
    #else
    return 0;
    #endif // USE_TORRENT
}

LRESULT CALLBACK UpdateDialog_t::NewButtonProc(HWND hWnd,UINT uMsg,WPARAM wParam,LPARAM lParam)
{
    // overrides button message procedures so it can capture mouse over
    // and display hover hints

    short x,y;

    x=LOWORD(lParam);
    y=HIWORD(lParam);

    switch(uMsg)
    {
        case WM_MOUSEMOVE:
            Popup->drawpopup(0,STR_UPD_BTN_THISPC_H,FLOATING_TOOLTIP,x,y,hWnd);
            ShowWindow(Popup->hPopup,SW_SHOWNOACTIVATE);
            if(!bMouseInWindow)
            {
                bMouseInWindow=1;
                TRACKMOUSEEVENT tme;
                tme.cbSize=sizeof(tme);
                tme.dwFlags=TME_LEAVE;
                tme.hwndTrack=hWnd;
                TrackMouseEvent(&tme);
            }
            break;

        case WM_MOUSELEAVE:
            bMouseInWindow=0;
            Popup->drawpopup(0,0,FLOATING_NONE,0,0,hWnd);
            break;

        default:
            return CallWindowProc(wpOrigButtonProc,hWnd,uMsg,wParam,lParam);
    }
    return true;
}

INT_PTR CALLBACK UpdateDialog_t::UpdateProcedure(HWND hwnd,UINT Message,WPARAM wParam,LPARAM lParam)
{
    // this is the dialog main message procedure

    LVCOLUMN lvc;
    HWND thispcbut,chk1;
    wchar_t buf[32];
    int i;
    static HBRUSH hBackgroundBrush;
    RECT r,rParent;

    thispcbut=GetDlgItem(hwnd,ID_UPD_CHECKTHISPC);
    chk1=GetDlgItem(hwnd,ID_UPD_ONLYUPDATES);

    switch(Message)
    {
        case WM_INITDIALOG:
            UpdateDialog.UpdateInProgress=false;
            // center to parent
            GetWindowRect(GetParent(hwnd),&rParent);
            GetWindowRect(hwnd,&r);
            r.left=rParent.left+(rParent.right-rParent.left)/2 - (r.right-r.left)/2;
            r.top=rParent.top+(rParent.bottom-rParent.top)/2 - (r.bottom-r.top)/2;
            if(r.left<0)r.left=0;
            if(r.top<0)r.top=0;
            SetWindowPos(hwnd,NULL,r.left,r.top,0,0, SWP_NOSIZE | SWP_NOZORDER);

            setMirroring(hwnd);
            ListView.init(hwnd);
            lvc.mask=LVCF_FMT|LVCF_WIDTH|LVCF_SUBITEM|LVCF_TEXT;
            lvc.pszText=const_cast<wchar_t *>(L"");
            for(i=0;i<6;i++)
            {
                lvc.cx=cxn[i];
                lvc.iSubItem=i;
                lvc.fmt=i?LVCFMT_RIGHT:LVCFMT_LEFT;
                ListView.InsertColumn(i,&lvc);
            }

            hUpdate=hwnd;


            wpOrigButtonProc=(WNDPROC)SetWindowLongPtr(thispcbut,GWLP_WNDPROC,(LONG_PTR)NewButtonProc);

            // theme
            hBackgroundBrush=CreateSolidBrush( D_C(MAINWND_INSIDE_COLOR) );

            // set initial keyboard focus to the start button (doesn't work)
            wParam=(WPARAM)GetDlgItem(hwnd,ID_UPD_BTN_UPDATE);
            return true;

        case WM_SHOWWINDOW:
            if(wParam==TRUE)
            {
                // i'm manually calling ResizeForm to set the layout for those
                // languages that need it
                r.bottom=410;r.left=0;r.right=440;r.top=0;
                MapDialogRect(hwnd,&r);
                UpdateDialog.ResizeForm(r.right-r.left,r.bottom-r.top);

                SetTimer(hwnd,1,200,nullptr);
                UpdateDialog.SetFonts();
                UpdateDialog.InitTexts();
                UpdateDialog.Populate(true);
                UpdateDialog.setCheckboxes();
                UpdateDialog.SetControls();

                // welcome dialog
                UpdateDialog.PerformAutoMode();

            }
            break;


        case WM_NOTIFY:
            {
                // a ListView item is changing
                // if the torrent is active i want to prevent changes to the list view
                LPNMHDR lpnmh = (LPNMHDR)lParam;
                if(lpnmh->code==LVN_ITEMCHANGING)
                {
                    LPNMLISTVIEW pnmv = (LPNMLISTVIEW)lParam;
                    // if torrent is running then abort the change
                    // block change only if the dialog is visible
                    // this allows me to close the dialog, reopen it and reset the
                    // checkboxes while the torrent is running
                    if(Updater&&!UpdateDialog.UpdateInProgress&&!Updater->IsPaused()&&IsWindowVisible(hUpdate))
                    {
                        SetWindowLongPtr(hwnd, DWLP_MSGRESULT, TRUE);
                        // Redraw the item so it maintains its original painted appearance
                        ListView_RedrawItems(lpnmh->hwndFrom, pnmv->iItem, pnmv->iItem);
                        InvalidateRect(hwnd, NULL, TRUE);
                        return true;
                    }
                }

                // a ListView item has changed - watch the count rise
                // as each item change is processed
                if(lpnmh->code==LVN_ITEMCHANGED)
                {
                    if(Updater&&!UpdateDialog.UpdateInProgress)
                    {
                        UpdateDialog.calctotalsize();
                        UpdateDialog.calcavailablespace();
                        UpdateDialog.UpdateTexts();
                    }
                    return true;
                }
                // column sort
                if(lpnmh->code==LVN_COLUMNCLICK)
                    if(lpnmh->idFrom==ID_UPD_LIST)
                    {
                        NMLISTVIEW* pListView = (NMLISTVIEW*)lParam;
                        // if the column is already sorted then reverse the sort
                        if(pListView->iSubItem==ListViewSortColumn)
                            ListViewSortAsc=!ListViewSortAsc;
                        else
                        {
                            ListViewSortColumn=pListView->iSubItem;
                            ListViewSortAsc=TRUE;
                        }
                        LPARAM lParamSort=ListViewSortColumn+1;
                        if(!ListViewSortAsc)lParamSort=-lParamSort;
                        SendMessage(ListView.hListg,LVM_SORTITEMS,lParamSort,(LPARAM)ListView.CompareFunc);
                        return TRUE;
                    }
                break;
            }

        case WM_DESTROY:
            SetWindowLongPtr(thispcbut,GWLP_WNDPROC,(LONG_PTR)wpOrigButtonProc);
            ListView.close();
            if(UpdateDialog.hDialogFont)
                DeleteObject(UpdateDialog.hDialogFont);
            if(UpdateDialog.hPromptFont)
                DeleteObject(UpdateDialog.hPromptFont);
            if(UpdateDialog.hButtonFont)
                DeleteObject(UpdateDialog.hButtonFont);
            return true;

        case WM_TIMER:
            KillTimer(hwnd,1);
            if(Settings.flags&FLAG_ONLYUPDATES)
                SendMessage(chk1,BM_SETCHECK,BST_CHECKED,0);

            // init the selection numbers
            UpdateDialog.calctotalsize();
            UpdateDialog.calcavailablespace();
            UpdateDialog.UpdateTexts();

            return true;

        case WM_COMMAND:
            switch(LOWORD(wParam))
            {

                case ID_UPD_BTN_UPDATE:
                    Updater->StartTorrent();
                    return TRUE;

                case ID_UPD_BTN_SHARE:
                    Updater->StartSpecialShare();
                    return true;

                case ID_UPD_BTN_STOP:
                    #ifdef USE_TORRENT
                    if(Torrents[activetorrent].handle.is_valid() &&
                       Torrents[activetorrent].handle.status().upload_mode)
                        Updater->StopSpecialShare();
                    else
                        Updater->StopTorrent();
                    #endif // USE_TORRENT
                    return true;

                case IDCANCEL:
                case ID_UPD_BTN_CLOSE:
                    hUpdate=nullptr;
                    EndDialog(hwnd,IDCANCEL);
                    break;

                case ID_UPD_ONLYUPDATES:
                    Settings.flags&=~FLAG_ONLYUPDATES;
                    if(SendMessage(chk1,BM_GETCHECK,0,0))Settings.flags|=FLAG_ONLYUPDATES;
                    UpdateDialog.Populate(true);
                    return true;

                case ID_UPD_KEEPSEEDING:
                    Settings.flags&=~FLAG_KEEPSEEDING;
                    if(SendMessage(GetDlgItem(hwnd,ID_UPD_KEEPSEEDING),BM_GETCHECK,0,0))Settings.flags|=FLAG_KEEPSEEDING;
                    return true;

                case ID_UPD_CHECKALL:
                case ID_UPD_UNCHECKALL:
                    for(i=0;i<ListView.GetItemCount();i++)
                        ListView.SetCheckState(i,LOWORD(wParam)==ID_UPD_CHECKALL?1:0);
                    return TRUE;

                case ID_UPD_CHECKTHISPC:
                    for(i=0;i<ListView.GetItemCount();i++)
                    {
                        *buf=0;
                        ListView.GetItemText(i,5,buf,32);
                        ListView.SetCheckState(i,StrStrIW(buf,STR(STR_UPD_YES))?1:0);
                    }
                    return TRUE;

                case ID_UPD_CHECKNETWORK:
                    for(i=0;i<ListView.GetItemCount();i++)
                    {
                        *buf=0;
                        ListView.GetItemText(i,0,buf,32);
                        bool chk=StrStrIW(buf,L"_Net_")||StrStrIW(buf,L"_LAN_")||StrStrIW(buf,L"_WLAN-WiFi_")||StrStrIW(buf,L"_WWAN-4G_")||StrStrIW(buf,L"Indexes");
                        ListView.SetCheckState(i,chk);
                    }
                default:
                    break;
            }
            break;

        case WM_CTLCOLORDLG:
            return (INT_PTR)hBackgroundBrush;
            break;

        case WM_CTLCOLORSTATIC:
            {
                // if not enough space to download turn label red
                bool avail=UpdateDialog.totalsize<UpdateDialog.totalavail;
                HDC hdc=(HDC)wParam;
                if((HWND)lParam==GetDlgItem(hUpdate,ID_UPD_TOTALAVAIL)&&!avail)
                {
                    SetTextColor(hdc, RGB(255,0,0));
                    SetBkColor(hdc, GetSysColor(COLOR_BTNFACE));
                    return (LRESULT)GetStockObject(HOLLOW_BRUSH);
                }
                // big user prompt
                else if((HWND)lParam==GetDlgItem(hUpdate,ID_UPD_PROMPT))
                {
                    // this is triggered by UpdatePromptMessage
                    SetTextColor(hdc, UpdateDialog.PromptColor);
                    SetBkMode(hdc, TRANSPARENT);
                    return (LRESULT)GetStockObject(HOLLOW_BRUSH);
                }

                // this applies the theme colours to the various dialog controls
                else if(
                        //(HWND)lParam==GetDlgItem(hUpdate,ID_UPD_PROGRESSBAR)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_SELECTION)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_SELECTED)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_TOTALSIZE)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_TOTALAVAIL)||

                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_STREAM_STATS)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_STREAM_STATE)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_STREAM_COMPLETE)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_STREAM_TIME)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_STREAM_SESSDL)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_STREAM_SESSUL)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_STREAM_SEEDS)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_STREAM_ALERT)||

                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_OPTIONS)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_ONLYUPDATES)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_KEEPSEEDING)||

                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_SPECIALMODE)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_TXT_SHARE)||
                        (HWND)lParam==GetDlgItem(hUpdate,ID_UPD_BTN_SHARE)
                       )
                       {
                            SetTextColor(hdc, D_C(MAINWND_TEXT_COLOR));
                            SetBkColor(hdc, D_C(MAINWND_INSIDE_COLOR));
                            return (LRESULT)hBackgroundBrush;
                       }

            }
            break;
        case WM_SIZING:
            {
                PRECT pRect = (PRECT)lParam;
                // Define minimum size constraints
                int minWidth = 676;
                int minHeight = 705;

                // Adjust the drag rectangle to enforce the minimum size
                if (pRect->right - pRect->left < minWidth)
                {
                    if (wParam == WMSZ_LEFT || wParam == WMSZ_TOPLEFT || wParam == WMSZ_BOTTOMLEFT)
                        pRect->left = pRect->right - minWidth;
                    else
                        pRect->right = pRect->left + minWidth;
                }

                if (pRect->bottom - pRect->top < minHeight)
                {
                    if (wParam == WMSZ_TOP || wParam == WMSZ_TOPLEFT || wParam == WMSZ_TOPRIGHT)
                        pRect->top = pRect->bottom - minHeight;
                    else
                        pRect->bottom = pRect->top + minHeight;
                }

                return TRUE; // Indicate that the application processed the message
                break;
            }
        case WM_SIZE:
            UpdateDialog.ResizeForm(LOWORD(lParam), HIWORD(lParam));
            InvalidateRect(hwnd, NULL, TRUE);
            return (INT_PTR)true;
            break;

        default:
            break;
    }
    return 0;
}

// Callback function to apply font to each control
BOOL CALLBACK inline SetChildFont(HWND hwndChild, LPARAM lParam) {
    HFONT hFont = (HFONT)lParam;
    SendMessage(hwndChild, WM_SETFONT, (WPARAM)hFont, TRUE);
    return TRUE;
}

void UpdateDialog_t::SetFonts()
{
    //
    // dialog font
    //

    // get the current font for the static text
    HFONT hFont = (HFONT)SendMessage(hUpdate, WM_GETFONT, 0, 0);

    // Note: If SendMessage returns NULL, the control is using the default system font.
    if (hFont == NULL)
        hFont = (HFONT)GetStockObject(DEFAULT_GUI_FONT); // Fallback to default system GUI font

    // Populate a LOGFONT structure with the current font settings
    LOGFONT lf;
    GetObject(hFont, sizeof(lf), &lf);


    // Modify the font attributes as desired
    //lf.lfWeight = FW_EXTRABOLD;               // Make text Bold
    int fz=D_X(FONT_SIZE)*120/100*Settings.scale/256;
    lf.lfHeight = fz;                    // Change font size
    std::wcscpy(lf.lfFaceName, D_STR(FONT_NAME));

    // Create the new font resource
    hDialogFont = CreateFontIndirect(&lf);

    // apply the new font
    SendMessage(hUpdate, WM_SETFONT, (WPARAM)hDialogFont, TRUE);

    // Enumerate and apply to all child buttons, inputs, labels, etc.
    EnumChildWindows(hUpdate, SetChildFont, (LPARAM)hDialogFont);




    //
    // the big user prompt
    //

    // 1. get the handle of the static control
    HWND hwnd=GetDlgItem(UpdateDialog.hUpdate,ID_UPD_PROMPT);
    if(!hwnd)
        return;

    // 2. get the current font for the static text
    hFont = (HFONT)SendMessage(hwnd, WM_GETFONT, 0, 0);

    // Note: If SendMessage returns NULL, the control is using the default system font.
    if (hFont == NULL)
        hFont = (HFONT)GetStockObject(DEFAULT_GUI_FONT); // Fallback to default system GUI font

    // 3. Populate a LOGFONT structure with the current font settings
    GetObject(hFont, sizeof(lf), &lf);

    // 4. Modify the font attributes as desired
    lf.lfWeight = FW_EXTRABOLD;               // Make text Bold
    lf.lfHeight = 20;                    // Change font size
    //wcscpy_s(lf.lfFaceName, L"Arial");   // Change font family

    // delete old font resource
    if(hPromptFont)
        DeleteObject(hPromptFont);

    // 5. Create the new font resource
    hPromptFont = CreateFontIndirect(&lf);

    // 6. Apply the new font to the static text control
    // and hopefully triggers WM_CTLCOLORSTATIC
    SendMessage(hwnd, WM_SETFONT, (WPARAM)hPromptFont, MAKELPARAM(TRUE, 0));
    SendMessage(hwnd, WM_SETTEXT, 0, (LPARAM)L"");
    InvalidateRect(UpdateDialog.hUpdate, NULL, TRUE);
    UpdateWindow(hwnd);

    // set the progress bar colours using theme
    // this is the same as the progress bar on the main screen
    SendMessage(GetDlgItem(hUpdate,ID_UPD_PROGRESSBAR), PBM_SETBARCOLOR, (WPARAM)0, D_C(PROGR_INSIDE_COLOR));
    SendMessage(GetDlgItem(hUpdate,ID_UPD_PROGRESSBAR), PBM_SETBKCOLOR, (WPARAM)0,  D_C(DRVITEM_INSIDE_COLOR_IU));



    //
    // start / stop / close buttons
    //

    // 1. get the handle of the static control
    hwnd=GetDlgItem(UpdateDialog.hUpdate,ID_UPD_BTN_UPDATE);
    if(!hwnd)
        return;

    // 2. get the current font for the static text
    hFont = (HFONT)SendMessage(hwnd, WM_GETFONT, 0, 0);

    // Note: If SendMessage returns NULL, the control is using the default system font.
    if (hFont == NULL)
        hFont = (HFONT)GetStockObject(DEFAULT_GUI_FONT); // Fallback to default system GUI font

    // 3. Populate a LOGFONT structure with the current font settings
    GetObject(hFont, sizeof(lf), &lf);

    // 4. Modify the font attributes as desired
    //lf.lfWeight = FW_EXTRABOLD;               // Make text Bold
    lf.lfHeight = 20;                    // Change font size
    //wcscpy_s(lf.lfFaceName, L"Arial");   // Change font family

    if(hButtonFont)
        DeleteObject(hButtonFont);

    // 5. Create the new font resource
    hButtonFont = CreateFontIndirect(&lf);

    // 6a. Apply the new font to the static text control
    // and hopefully triggers WM_CTLCOLORSTATIC
    SendMessage(hwnd, WM_SETFONT, (WPARAM)hButtonFont, MAKELPARAM(TRUE, 0));
    SendMessage(hwnd, WM_SETTEXT, 0, (LPARAM)L"");
    InvalidateRect(UpdateDialog.hUpdate, NULL, TRUE);
    UpdateWindow(hwnd);

    // 6b. Apply the new font to the static text control
    // and hopefully triggers WM_CTLCOLORSTATIC
    hwnd=GetDlgItem(UpdateDialog.hUpdate,ID_UPD_BTN_STOP);
    SendMessage(hwnd, WM_SETFONT, (WPARAM)hButtonFont, MAKELPARAM(TRUE, 0));
    SendMessage(hwnd, WM_SETTEXT, 0, (LPARAM)L"");
    InvalidateRect(UpdateDialog.hUpdate, NULL, TRUE);
    UpdateWindow(hwnd);

    // 6c. Apply the new font to the static text control
    // and hopefully triggers WM_CTLCOLORSTATIC
    hwnd=GetDlgItem(UpdateDialog.hUpdate,ID_UPD_BTN_CLOSE);
    SendMessage(hwnd, WM_SETFONT, (WPARAM)hButtonFont, MAKELPARAM(TRUE, 0));
    SendMessage(hwnd, WM_SETTEXT, 0, (LPARAM)L"");
    InvalidateRect(UpdateDialog.hUpdate, NULL, TRUE);
    UpdateWindow(hwnd);

}
#ifdef USE_TORRENT
std::wstring UpdaterImp::TorrentStateStr(libtorrent::torrent_status::state_t state)
{
    // translate the torrent state enum to a readable string
    switch(state)
    {
        case libtorrent::torrent_status::state_t::checking_files:
             return STR(STR_TR_ST1);
            break;
        case libtorrent::torrent_status::state_t::downloading_metadata:
            return STR(STR_TR_ST2);
            break;
        case libtorrent::torrent_status::state_t::downloading:
            return STR(STR_TR_ST3);
            break;
        case libtorrent::torrent_status::state_t::finished:
            if(Updater->IsSeedingDrivers())
                return STR(STR_TR_ST5);
            else
                return STR(STR_TR_ST4);
            break;
        case libtorrent::torrent_status::state_t::seeding:
            return STR(STR_TR_ST5);
            break;
// deprecated
//        case libtorrent::torrent_status::state_t::queued_for_checking:
//            state=STR(STR_TR_ST6);
//            break;
        case libtorrent::torrent_status::state_t::checking_resume_data:
            return STR(STR_TR_ST7);
            break;
        default:
            return L"Unknown";
            break;
    }
}
#endif

void UpdateDialog_t::UpdatePromptMessage()
{
    // this updates the big user prompt and progress bar

    HWND hwnd=GetDlgItem(UpdateDialog.hUpdate,ID_UPD_PROMPT);
    if(!hwnd)
        return;

    #ifdef USE_TORRENT
    HWND hwndp=GetDlgItem(UpdateDialog.hUpdate,ID_UPD_PROGRESSBAR);

    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    bool TorrentIsValid=CurrentTorrent.is_valid();

    wchar_t buf[BUFLEN];
    wchar_t num3[BUFLEN],num4[BUFLEN];
    std::wstring msg=L"";
    std::wstring ws1,ws2=L"";
    int perc=0;


    if(TorrentIsValid)
    {
        lt::torrent_status st=CurrentTorrent.status();

        // used in WM_CTLCOLORSTATIC
        PromptColor=D_C(DRVITEM_TEXT1_COLOR_IU);

        // translate the torrent state to a user friendly message

        // this is the special seed mode of the drivers directory
        if(st.upload_mode&&!(st.state==libtorrent::torrent_status::state_t::checking_files))
        {
            SendMessage(GetDlgItem(hUpdate,ID_UPD_PROGRESSBAR), PBM_SETPOS, (WPARAM)100, 0);
            format_size(num3,st.upload_rate,1);
            format_size(num4,st.total_upload,0);
            wsprintf(buf,STR(STR_DWN_SEEDING),num4,num3);
            msg=buf;

        }
        else
        {

            switch(st.state)
            {
                case libtorrent::torrent_status::state_t::checking_files:
                    perc=st.progress*100;
                    SendMessage(GetDlgItem(hUpdate,ID_UPD_PROGRESSBAR), PBM_SETPOS, (WPARAM)perc, 0);
                    wsprintf(buf,STR(STR_UPD_PROMPT_01),perc);
                    msg=buf;
                    break;
                case libtorrent::torrent_status::state_t::downloading:
                    if(st.total_wanted>0)
                    {
                        perc=st.total_wanted_done*100/st.total_wanted;
                        SendMessage(GetDlgItem(hUpdate,ID_UPD_PROGRESSBAR), PBM_SETPOS, (WPARAM)perc, 0);
                    }
                    //wsprintf(buf,STR(STR_UPD_PROMPT_02),perc);
                    ws1=BytesToStr(st.total_wanted_done);
                    ws2=BytesToStr(st.total_wanted);
                    wsprintf(buf,STR(STR_UPD_PROGRES),ws1.c_str(),ws2.c_str(),perc);
                    msg=buf;
                    break;
                case libtorrent::torrent_status::state_t::seeding:
                    msg=STR(STR_UPD_PROMPT_03);
                    break;
                case libtorrent::torrent_status::state_t::finished:
                    // get the progress bar to 100%
                    if(st.total_wanted)
                        perc=st.total_wanted_done*100/st.total_wanted;
                    SendMessage(GetDlgItem(hUpdate,ID_UPD_PROGRESSBAR), PBM_SETPOS, (WPARAM)perc, 0);
                    if(Updater->IsSeedingDrivers())
                        msg=STR(STR_UPD_PROMPT_03);
                    else
                        msg=STR(STR_UPD_PROMPT_04);
                    break;
                case libtorrent::torrent_status::state_t::checking_resume_data:
                    msg=STR(STR_UPD_PROMPT_05);
                    break;
                default:
                    if(CurrentTorrent.status().upload_mode)
                        msg=STR(STR_UPD_PROMPT_03);
                    else
                        msg=Updater->TorrentStateStr(st.state);
                    break;
            }
        }

        // special cases will override torrent state
        if(IsStartingShareMode)
            msg=STR(STR_UPD_PROMPT_10);
        else if(IsEndingShareMode)
            msg=STR(STR_UPD_PROMPT_11);
        else if(IsInitializing)
            msg=STR(STR_INITIALIZING);
        else if(IsMovingFiles)
            msg=STR(STR_UPD_PROMPT_02);
        else if (IsFlushing)
            msg=STR(STR_DWN_CLOSING);
        else if(Updater->IsPaused())
        {
            SendMessage(GetDlgItem(hUpdate,ID_UPD_PROGRESSBAR), PBM_SETPOS, (WPARAM)0, 0);
            int itemcount=ListView_GetItemCount(ListView.hListg);
            if(itemcount==0)
                msg=STR(STR_UPD_PROMPT_06);
            else
                msg=STR(STR_UPD_PROMPT_07);
        }
    }
    else
    {
        msg=STR(STR_UPD_PROMPT_08);
    }

    SendMessage(hwnd, WM_SETREDRAW, FALSE, 0);
    // this will trigger WM_CTLCOLORSTATIC
    SetWindowText(hwnd,msg.c_str());
    SendMessage(hwnd, WM_SETREDRAW, TRUE, 0);

    InvalidateRect(hwndp,NULL,true);
    InvalidateRect(hwnd,NULL,true);
    UpdateWindow(hwndp);
    #endif // USE_TORRENT

}

void UpdateDialog_t::UpdateTorrentAlert(const std::string msg)
{
    if(Updater->torrentalerts)
    {
        // the torrent alert status message at the bottom of the stats section
        wchar_t buf[BUFLEN];

        std::wstring ws1=STR(STR_UPD_STREAM_ALERT);
        std::wstring ws2=utf8_to_wstring(msg);
        wsprintf(buf,ws1.c_str(),ws2.c_str());
        SetWindowText(GetDlgItem(UpdateDialog.hUpdate,ID_UPD_STREAM_ALERT),buf);
    }
}

void UpdateDialog_t::UpdateTorrentStats()
{
    // the torrents stats section

    #ifdef USE_TORRENT

    wchar_t num2[BUFLEN];
    std::wstring state=L"";
    std::wstring ws1,ws2,ws3,ws4,ws5;
    wchar_t buf[BUFLEN];
    std::int64_t remaining;

    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;

    // torrent progress is 0 - 1.0
    lt::torrent_status st=CurrentTorrent.status();
    int perc=st.progress*100;

    state=Updater->TorrentStateStr(st.state);

    if(Updater->IsPaused())
        state=STR(STR_TR_ST9);
    if(IsMovingFiles)
        state=STR(STR_TR_ST8);
    if(st.upload_mode)
        state=STR(STR_TR_ST5);
    // state
    state=STR(STR_UPD_STREAM_STATE)+state;
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_STATE),state.c_str());

    // complete
    ws1=STR(STR_UPD_STREAM_COMPLETE);
    ws2=BytesToStr(st.total_wanted_done);
    ws3=BytesToStr(st.total_wanted);
    wsprintf(buf,ws1.c_str(),ws2.c_str(),ws3.c_str(),perc);
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_COMPLETE),buf);

    // elapsed
    remaining=0;
    if(TorrentStartTime>0)
    {
        if(st.download_rate)
        {
            AverageSpeed=static_cast<int>(SMOOTHING_FACTOR*st.download_rate+(1-SMOOTHING_FACTOR)*AverageSpeed);
            if(AverageSpeed)remaining=(st.total_wanted-st.total_wanted_done)/AverageSpeed*1000;
        }
    }

    // remaining
    ws1=STR(STR_UPD_STREAM_TIME);
    format_time(num2,remaining);
    wsprintf(buf,ws1.c_str(),num2);
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_TIME),buf);

    // session downloads
    ws1=STR(STR_UPD_STREAM_SESSDL);
    ws2=BytesToStr(st.total_download);
    ws3=BytesToStr(st.download_rate);
    wsprintf(buf,ws1.c_str(),ws2.c_str(),ws3.c_str());
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_SESSDL),buf);

    // session uploads
    ws1=STR(STR_UPD_STREAM_SESSUL);
    ws2=BytesToStr(st.total_upload);
    ws3=BytesToStr(st.upload_rate);
    wsprintf(buf,ws1.c_str(),ws2.c_str(),ws3.c_str());
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_SESSUL),buf);

    // seeds / peers
    ws1=STR(STR_UPD_STREAM_SEEDS);
    wsprintf(buf,ws1.c_str(),st.num_seeds,st.list_seeds,st.num_peers,st.list_peers);
    SetWindowText(GetDlgItem(hUpdate,ID_UPD_STREAM_SEEDS),buf);

    // don't know what this does - might be the hover hint
    MainWindow.redrawfield();

    // the big user prompt
    UpdatePromptMessage();
    System.ProcessMessages();

    // update the front end but only if the torrent is active
    if(!Updater->IsPaused())
    {
        // this updates the progress bar on the front end - not the text
        // seems to be parts of 1000
        manager_g->itembar_settext(SLOT_DOWNLOAD,1,nullptr,-1,-1,st.progress*1000);

    }

    #endif // USE_TORRENT
}

int UpdateDialog_t::Populate(bool reload)
{
    // populate the list view

    #ifdef USE_TORRENT
    std::string fp;
    std::wstring SourceFile;
    std::wstring DestFile;
    std::wstring IndexDir={Settings.index_dir};
    std::wstring DrpDir={Settings.drp_dir};

    wchar_t buf[BUFLEN];
    int ret=0;
    LocalRevision=0;
    TorrentRevision=0;

    libtorrent::error_code ec;

    // using the current active torrent
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return 0;

    // don't talk while I'm interrupting
    if(ListViewUpdating)
        return 0;

    ListViewUpdating=true;

    // if I'm in special share mode then use the alternative populate2
    if(CurrentTorrent.status().upload_mode)
    {
        Populate2();
        ListViewUpdating=false;
        return 0;
    }

    // Read torrent info
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();
    libtorrent::torrent_status status=CurrentTorrent.status(status_flags_t::all());

    // get file progress
    std::vector<std::int64_t> file_progress;
    CurrentTorrent.file_progress(file_progress,false);

    // Calculate size and progress for the app and indexes
    int missingindexes=0;
    int newver=0;
    int64_t basesize,basedownloaded=0;
    int64_t indexsize,indexdownloaded=0;


    //
    // check for missing online indexes
    //
    indexsize=0; indexdownloaded=0;
    for(int i=0;i<info->num_files();i++)
    {
        fp=fs.file_path(i);
        std::string fn=to_lower(fp);
        if(fn.find("sdio_update\\indexes")!=std::string::npos)
        {
            DestFile=utf8_to_wstring(fp.substr(fp.rfind("\\")+1));
            DestFile=L"_"+DestFile.substr(1);
            DestFile=IndexDir + L"\\" + DestFile;
            if(!System.FileExists(DestFile.c_str()))
                missingindexes=1;
            indexsize+=fs.file_size(i);
            indexdownloaded+=file_progress[i];
        }
    }


    //
    // check for missing application files
    //
    basesize=0; basedownloaded=0;
    for(int i=0;i<info->num_files();i++)
    {
        fp=fs.file_path(i);
        std::string fn=to_lower(fp);
        if(  (fn.find("sdio_update\\indexes")==std::string::npos) &&
             (fn.find("sdio_update\\drivers")==std::string::npos) )
        {
            // the file name minus the parent directory 'SDIO_Update'
            fp=fp.substr(fp.find("\\")+1);
            size_t r=to_lower(fp).find("sdio_r");
            if(r!=std::string::npos)
                TorrentRevision=atol(fp.substr(r+6).c_str());
            basesize+=fs.file_size(i);
            basedownloaded+=file_progress[i];
        }
    }






    // Disable redrawing of the list
    UpdateInProgress=true;

    // try to reduce screen refreshes
    if(reload)
    {
        SendMessage(ListView.hListg,WM_SETREDRAW,false,0);
        // clear the list view
        if(reload)
            SendMessage(ListView.hListg,LVM_DELETEALLITEMS,0,0L);
    }

    // Setup LVITEM
    LVITEM lvI;
    lvI.mask      =LVIF_TEXT|LVIF_STATE|LVIF_PARAM;
    lvI.stateMask =0;
    lvI.iSubItem  =0;
    lvI.state     =0;
    lvI.iItem     =0;

    int row=0;
    #ifdef _WIN64
    LocalRevision=System.FindLatestExeVersion(64);
    #else
    LocalRevision=System.FindLatestExeVersion(32);
    #endif // _WIN64

    // only return the torrent exe version if it's newer than what i have
    if(TorrentRevision>LocalRevision)ret+=TorrentRevision<<8;

    // the application entry
    if(TorrentRevision>LocalRevision&&ListView.IsVisible())
    {
        // the data item
        type_item *ItemData=new type_item;
        lvI.lParam=(LPARAM)ItemData;
        // DefaultSort enables me to put app and indexes at the top of the list
        // followed by sorted drivers when sort column is 666
        ItemData->DefaultSort=-2;
        wcscpy(ItemData->ItemName,STR(STR_UPD_APP));
        ItemData->SizeMB=basesize/1024/1024;
        ItemData->Percent=basedownloaded*100/basesize;
        ItemData->VersionNew=TorrentRevision;
        ItemData->VersionCurrent=LocalRevision;
        wcscpy(ItemData->ForThisPC,STR(STR_UPD_YES));
        // the list item
        lvI.pszText=const_cast<wchar_t *>(STR(STR_UPD_APP));
        if(reload)
            row=ListView.InsertItem(&lvI);
        wsprintf(buf,L"%d %s",(int)(basesize/1024/1024),STR(STR_UPD_MB));
        ListView.SetItemTextUpdate(row,1,buf);
        wsprintf(buf,L"%d%%",(int)(basedownloaded*100/basesize));
        ListView.SetItemTextUpdate(row,2,buf);
        wsprintf(buf,L" SDIO_R%d",TorrentRevision);
        ListView.SetItemTextUpdate(row,3,buf);
        wsprintf(buf,L" SDIO_R%d",LocalRevision);
        ListView.SetItemTextUpdate(row,4,buf);
        ListView.SetItemTextUpdate(row,5,STR(STR_UPD_YES));
        row++;
    }

    // Add indexes to the list
    if(missingindexes&&ListView.IsVisible())
    {
        // the data item
        type_item *ItemData=new type_item;
        lvI.lParam=(LPARAM)ItemData;
        ItemData->DefaultSort=-1;
        wcscpy(ItemData->ItemName,STR(STR_UPD_INDEXES));
        ItemData->SizeMB=indexsize/1024/1024;
        ItemData->Percent=indexdownloaded*100/indexsize;
        ItemData->VersionNew=0;
        ItemData->VersionCurrent=0;
        wcscpy(ItemData->ForThisPC,STR(STR_UPD_YES));
        // the list item
        lvI.pszText=const_cast<wchar_t *>(STR(STR_UPD_INDEXES));
        if(reload)
            row=ListView.InsertItem(&lvI);
        wsprintf(buf,L"%d %s",(int)(indexsize/1024/1024),STR(STR_UPD_MB));
        ListView.SetItemTextUpdate(row,1,buf);
        wsprintf(buf,L"%d%%",(int)(indexdownloaded*100/indexsize));
        ListView.SetItemTextUpdate(row,2,buf);
        ListView.SetItemTextUpdate(row,5,STR(STR_UPD_YES));
        row++;
    }


    // Add driverpacks to the list
    for(int i=0;i<info->num_files();i++)
    {
        char *filename=nullptr;
        char *filenamefull=nullptr;
        fp=fs.file_path(i);
        filenamefull=strchr(fp.c_str(),'\\')+1;
        filename=strchr(filenamefull,'\\');
        if(filename)filename=strchr(filenamefull,'\\')+1;
        else filename=filenamefull;

        if(StrStrIA(filenamefull,".7z"))
        {
            int oldver;

            wsprintf(buf,L"%S",filename);
            lvI.pszText=buf;
            int sz=(int)(fs.file_size(i)/1024/1024);
            if(!sz)sz=1;

            // extract the ver from the file name in the torrent
            newver=getnewver(filenamefull);
            // get the file name prior to the ver and see if that is in the drp directory
            // and extract the ver from that file name, 0=not found
            oldver=getcurver(filename);

            // this flag means only get new versions of packs i already have
            if(Settings.flags&FLAG_ONLYUPDATES)
                {if(newver>oldver&&oldver)ret++;else continue;}
            else
                if(newver>oldver)ret++;

            if(newver>oldver)     //&&ListView.IsVisible())
            {
                // the data item
                type_item *ItemData=new type_item;
                lvI.lParam=(LPARAM)ItemData;
                ItemData->DefaultSort=i;
                wcscpy(ItemData->ItemName,buf);
                ItemData->SizeMB=sz;
                int fileprogress=file_progress[i]*100/fs.file_size(i);
                ItemData->Percent=fileprogress;;
                ItemData->VersionNew=newver;
                ItemData->VersionCurrent=oldver;
                wcscpy(ItemData->ForThisPC,STR(STR_UPD_YES+manager_g->manager_drplive(buf)));
                // the list item
                if(reload)
                    row=ListView.InsertItem(&lvI);
                wsprintf(buf,L"%d %s",sz,STR(STR_UPD_MB));
                ListView.SetItemTextUpdate(row,1,buf);
                wsprintf(buf,L"%d%%",fileprogress);
                ListView.SetItemTextUpdate(row,2,buf);
                wsprintf(buf,L"%d",newver);
                ListView.SetItemTextUpdate(row,3,buf);
                wsprintf(buf,L"%d",oldver);
                if(!oldver)wsprintf(buf,L"%ws",STR(STR_UPD_MISSING));
                ListView.SetItemTextUpdate(row,4,buf);
                // this pc
                wsprintf(buf,L"%S",filename);
                wsprintf(buf,L"%ws",STR(STR_UPD_YES+manager_g->manager_drplive(buf)));
                ListView.SetItemTextUpdate(row,5,buf);
                row++;
            }
        }

    }

    // Enable redrawing of the list
    UpdateInProgress=false;

    // preselect the first item in the list
    ListView.SetItemState(0,LVIS_FOCUSED|LVIS_SELECTED,LVIS_FOCUSED|LVIS_SELECTED);

    if(!reload)
    {
        ListViewUpdating=false;
        return ret;
    }

    // try to reduce flickering
    if(reload)
    {
        SendMessage(ListView.hListg,WM_SETREDRAW,true,0);
        // sort the list items - 667 resets the sort column
        // the sort compare function does -1 on the column number, dunno why
        SendMessage(ListView.hListg,LVM_SORTITEMS,667,(LPARAM)ListView.CompareFunc);
    }

    // update the front end
    if(!ret)manager_g->itembar_settext(SLOT_NOUPDATES,0);
    manager_g->itembar_settext(SLOT_DOWNLOAD,ret?1:0,nullptr,ret,0,0);

    ListViewUpdating=false;
    return ret;
    #else
    UNREFERENCED_PARAMETER(reload);
    return 0;
    #endif // USE_TORRENT
}

void UpdateDialog_t::Populate2()
{
    // populate the list view when Special Share Mode is running

    #ifdef USE_TORRENT

    wchar_t buf[BUFLEN];
    int row=0;
    int cnt=0;
    LocalRevision=0;
    TorrentRevision=0;

    libtorrent::error_code ec;

    // using the current active torrent
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;

    // Read torrent info
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();
    libtorrent::torrent_status status=CurrentTorrent.status(status_flags_t::all());


    UpdateInProgress=true;
    SendMessage(ListView.hListg,WM_SETREDRAW,false,0);

    // clear the list view
    SendMessage(ListView.hListg,LVM_DELETEALLITEMS,0,0L);

    // Setup LVITEM
    LVITEM lvI;
    lvI.mask      =LVIF_TEXT|LVIF_STATE|LVIF_PARAM;
    lvI.stateMask =0;
    lvI.iSubItem  =0;
    lvI.state     =0;
    lvI.iItem     =0;




    // iterate the torrent


    // add driver entries to the list view from the drp_dir directory
    // this assumes StartSpecialShare has already set the priorities
    // according to which files exist

    for(int i=0;i<info->num_files();i++)
    {
        if(CurrentTorrent.file_priority(i)==1)
        {
            //char *filename=nullptr;
            std::string fp=std::string(fs.file_name(i));
            //filename=strchr(fp.c_str(),'\\')+1;

            // file size
            int sz=(int)(fs.file_size(i)/1024/1024);
            if(!sz)sz=1;

            // the data item
            type_item *ItemData=new type_item;
            lvI.lParam=(LPARAM)ItemData;
            ItemData->DefaultSort=i;

            wsprintf(buf,L"%S",fp.c_str());
            wcscpy(ItemData->ItemName,buf);
            lvI.pszText=buf;

            // the list item
            row=ListView.InsertItem(&lvI);
            wsprintf(buf,L"%d %s",sz,STR(STR_UPD_MB));
            ListView.SetItemTextUpdate(row,1,buf);

            int oldver=getcurver(fp.c_str());
            wsprintf(buf,L"%d",oldver);
            ListView.SetItemTextUpdate(row,3,buf);
            ListView.SetItemTextUpdate(row,4,buf);

            cnt++;
        }

    }


    SendMessage(ListView.hListg,WM_SETREDRAW,true,0);
    InvalidateRect(ListView.hListg,NULL,true);
    UpdateWindow(ListView.hListg);
    UpdateDialog.setCheckboxes();
    UpdateDialog.calctotalsize();
    UpdateDialog.calcavailablespace();
    UpdateDialog.UpdateTexts();
    UpdateInProgress=false;

    #endif // USE_TORRENT
}

void UpdateDialog_t::UpdateProgress()
{
    // this updates the percent column in the list view

    #ifdef USE_TORRENT

    wchar_t buf[BUFLEN];
    int64_t indexsize;
    int64_t indexdownloaded;
    int64_t appsize;
    int64_t appdownloaded;
    std::wstring wfilename;
    int fileprogress;
    const char *indexfilename="Indexes";
    const char *applicationfilename="Application";

    // using the current active torrent
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;

    indexsize=0;
    indexdownloaded=0;
    appsize=0;
    appdownloaded=0;


    // Read torrent info
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();
    libtorrent::torrent_status status=CurrentTorrent.status(status_flags_t::all());

    // read torrent progress info
    std::vector<std::int64_t> file_progress;
    CurrentTorrent.file_progress(file_progress,1);

    // iterate the torrent files
    for(int j=0; j<info->num_files(); j++)
    {
        // get the file info
        //char *filename=nullptr;
        //char *filenamefull=nullptr;
        std::string fp=fs.file_path(j);
        char *filenamefull=strchr(fp.c_str(),'\\')+1;
        char *filename=strchr(filenamefull,'\\');
        if(filename)filename=strchr(filenamefull,'\\')+1;
        else filename=filenamefull;

        // add up everything in the torrent indexes directory
        if(StrStrIA(filenamefull,"indexes\\"))
        {
            indexsize+=fs.file_size(j);
            indexdownloaded+=file_progress[j];
            fileprogress=indexdownloaded*100/indexsize;
            strcpy(filename,indexfilename);
        }
        // add up driver packs
        else if(StrStrIA(filenamefull,".7z"))
        {
            fileprogress=file_progress[j]*100/fs.file_size(j);
        }
        // add up all other files in the torrent (exe,docs etc)
        else
        {
            appsize+=fs.file_size(j);
            appdownloaded+=file_progress[j];
            fileprogress=appdownloaded*100/appsize;
            strcpy(filename,applicationfilename);
        }


        // iterate the list view items
        // looking for the current torrent file in the list view
        wfilename=utf8_to_wstring(filename);
        for(int i=0; i<ListView.GetItemCount(); i++)
        {
            // get the list item text
            *buf=0;
            ListView.GetItemText(i, 0, buf, BUFLEN);

            // is this the droid i'm looking for?
            if(wcscmp(wfilename.c_str(),buf)==0)
            {
                // update the percent column
                wsprintf(buf,L"%d%%",fileprogress);
                ListView.SetItemTextUpdate(i,2,buf);
                break;
            }
        }

    }
    #endif // USE_TORRENT
}

void UpdateDialog_t::SetControls()
{
    // set the form controls enabled status according to the state of the current torrent

    #ifdef USE_TORRENT

    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;

    // get the torrent paused state
    bool IsPaused = Updater->IsPaused();
    wchar_t spec[BUFLEN];

    wcscpy(spec,Settings.drp_dir);
    wcscat(spec,L"\\DP_*.7z");

    EnableWindow(GetDlgItem(hUpdate, ID_UPD_CHECKALL),IsPaused);
    EnableWindow(GetDlgItem(hUpdate, ID_UPD_UNCHECKALL),IsPaused);
    EnableWindow(GetDlgItem(hUpdate, ID_UPD_CHECKNETWORK),IsPaused);
    EnableWindow(GetDlgItem(hUpdate, ID_UPD_CHECKTHISPC),IsPaused);

    //EnableWindow(GetDlgItem(hUpdate, ID_UPD_OPTIONS),IsPaused);
    EnableWindow(GetDlgItem(hUpdate, ID_UPD_ONLYUPDATES),IsPaused && System.FileExists2(spec));
    EnableWindow(GetDlgItem(hUpdate, ID_UPD_KEEPSEEDING), !CurrentTorrent.status().upload_mode);

    // hide the share mode until some driver packs have been downloaded
    if(System.FileExists2(spec))
    {
        ShowWindow(GetDlgItem(hUpdate, ID_UPD_SPECIALMODE), SW_SHOW);
        ShowWindow(GetDlgItem(hUpdate, ID_UPD_TXT_SHARE), SW_SHOW);
        ShowWindow(GetDlgItem(hUpdate, ID_UPD_BTN_SHARE), SW_SHOW);
    }
    else
    {
        ShowWindow(GetDlgItem(hUpdate, ID_UPD_SPECIALMODE), SW_HIDE);
        ShowWindow(GetDlgItem(hUpdate, ID_UPD_TXT_SHARE), SW_HIDE);
        ShowWindow(GetDlgItem(hUpdate, ID_UPD_BTN_SHARE), SW_HIDE);
    }

    EnableWindow(GetDlgItem(hUpdate, ID_UPD_BTN_SHARE),IsPaused);

    EnableWindow(GetDlgItem(hUpdate, ID_UPD_BTN_UPDATE),IsPaused);
    EnableWindow(GetDlgItem(hUpdate, ID_UPD_BTN_STOP),!IsPaused);

    // disable buttons if not enough space
    if(UpdateDialog.totalsize>UpdateDialog.totalavail)
        EnableWindow(GetDlgItem(hUpdate, ID_UPD_BTN_UPDATE),0);

    #endif // USE_TORRENT
}

void UpdateDialog_t::SetAllDriverCheckboxes()
{
    // this triggers the item changed message for each item
    SendMessage(hUpdate,WM_COMMAND,ID_UPD_CHECKALL,0);
}

void UpdateDialog_t::SetCheckbox(int id,int checked)
{
    // set a checkbox control on the dialog
    SendMessage(GetDlgItem(UpdateDialog.hUpdate,id),BM_SETCHECK, checked,0);
}

void UpdateDialog_t::OpenDialog(int automode)
{
    #ifdef USE_TORRENT
    // triggers the init message
    UpdateDialog.AutoMode=automode;
    DialogBox(ghInst,
              MAKEINTRESOURCE(IDD_DIALOG2),
              MainWindow.hMain,
              (DLGPROC)UpdateProcedure);
    #else
    UNREFERENCED_PARAMETER(automode);
    #endif // USE_TORRENT
}
//}

/*
 *
 *                        END UPDATE DIALOG
 *
*/

//{ Updater
void UpdaterImp::ShowPopup(Canvas &canvas)
{
    // mouse-over hover hint on the download slot
    // called from main form

    #ifdef USE_TORRENT

    wchar_t num1[BUFLEN],num2[BUFLEN];
    std::wstring state=L"";
    std::int64_t elapsed,remaining=0;

    if(!hSession)
        return;

    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;

    textdata_vert td(canvas);
    int p0=D_X(POPUP_OFSX),p1=D_X(POPUP_OFSX)+10;
    long long per=0;
    elapsed=0;

    td.y=D_X(POPUP_OFSY);

    torrent_status st=CurrentTorrent.status();

    format_size(num1,st.total_wanted_done,0);
    format_size(num2,st.total_wanted,0);

    if(st.total_wanted)
        per=st.total_wanted_done*100/st.total_wanted;
    td.TextOutSF(STR(STR_DWN_DOWNLOADED),STR(STR_DWN_DOWNLOADED_F),num1,num2,per);

    format_size(num1,st.total_upload,0);
    td.TextOutSF(STR(STR_DWN_UPLOADED),num1);

    if(TorrentStartTime>0)
    {
        elapsed=System.GetTickCountWr()-TorrentStartTime;
        if(st.download_rate)
        {
            AverageSpeed=static_cast<int>(SMOOTHING_FACTOR*st.download_rate+(1-SMOOTHING_FACTOR)*AverageSpeed);
            if(AverageSpeed)remaining=(st.total_wanted-st.total_wanted_done)/AverageSpeed*1000;
        }
    }

    format_time(num1,elapsed);
    format_time(num2,remaining);

    td.TextOutSF(STR(STR_DWN_ELAPSED),num1);
    td.TextOutSF(STR(STR_DWN_REMAINING),num2);

    // torrent state
    td.nl();
    state=Updater->TorrentStateStr(st.state);
    if(IsPaused())
        state=STR(STR_TR_ST9);
    if(IsMovingFiles)
        state=STR(STR_TR_ST8);

    // torrent reporting an error
    td.TextOutSF(STR(STR_DWN_STATUS),L"%s",state.c_str());
    if(st.errc)
    {
        std::string err=st.errc.message();
        state=utf8_to_wstring(err);
        if(state.length()>0)
        {
            td.col=D_C(POPUP_CMP_INVALID_COLOR);
            td.TextOutSF(STR(STR_DWN_ERROR),L"%s",state.c_str());
            td.col=D_C(POPUP_TEXT_COLOR);
        }
    }

    // speeds
    format_size(num1,st.download_rate,1);
    td.TextOutSF(STR(STR_DWN_DOWNLOADSPEED),num1);
    format_size(num1,st.upload_rate,1);
    td.TextOutSF(STR(STR_DWN_UPLOADSPEED),num1);

    td.nl();
    td.TextOutSF(STR(STR_DWN_SEEDS),STR(STR_DWN_SEEDS_F),st.num_seeds,st.list_seeds);
    td.TextOutSF(STR(STR_DWN_PEERS),STR(STR_DWN_SEEDS_F),st.num_peers,st.list_peers);
    format_size(num1, st.total_redundant_bytes,0);
    format_size(num2,st.total_failed_bytes,0);
    td.TextOutSF(STR(STR_DWN_WASTED),STR(STR_DWN_WASTED_F),num1,num2);

    Popup->popup_resize((int)(td.getMaxsz()+POPUP_SYSINFO_OFS+p0+p1),td.y+D_X(POPUP_OFSY));

    #else
    UNREFERENCED_PARAMETER(canvas);
    #endif // USE_TORRENT

}

void UpdaterImp::RemoveDriverPack(const wchar_t *ptr)
{
    // deletes a driver pack file from the drivers directory
    wchar_t buf[BUFLEN];
    wsprintf(buf,L"%ws\\%ws",Settings.drp_dir,ptr);

    if(System.FileExists2(buf))
    {
        WStringShort buf2;
        buf2.sprintf(L"/c del %ws\\%ws",Settings.drp_dir,ptr);
        Log.print_con("Deleting %S\n",buf);
        System.run_command(L"cmd",buf2.Get(),SW_HIDE,1);
    }
}

void UpdaterImp::RemoveRedundantDriverPacks()
{
    // these files were used once but no longer
    // and should be removed
    RemoveDriverPack(L"DP_SDIO_*.7z");
    RemoveDriverPack(L"DP_SDIO2_*.7z");
    RemoveDriverPack(L"DP_SDIO3_*.7z");
    RemoveDriverPack(L"DP_SDIO4_*.7z");
    RemoveDriverPack(L"DP_SDIO5_*.7z");
    RemoveDriverPack(L"DP_SDIO6_*.7z");
    RemoveDriverPack(L"DP_Sound_ADI_*.7z");
    RemoveDriverPack(L"DP_Video_Intel_DCH_*.7z");
    RemoveDriverPack(L"DP_Video_Intel_DCH2x_*.7z");
    RemoveDriverPack(L"DP_Video_nVIDIA_DCH_*.7z");
    RemoveDriverPack(L"DP_Videos_AMD_DCH_*.7z");
    RemoveDriverPack(L"DP_Videos_AMD-10_*.7z");
    RemoveDriverPack(L"DP_yEXP_*.7z");
    RemoveDriverPack(L"DP_zVirtual_*.7z");
}

void UpdaterImp::RemoveOldDriverpacks(const wchar_t *ptr)
{
    // this removes older driver pack files
    // where an updated one exists
    WStringShort bffw;
    bffw.append(ptr);
    wchar_t *s=bffw.GetV();
    while(*s)
    {
        if(*s=='_'&&s[1]>='0'&&s[1]<='9')
        {
            *s=0;
            s=const_cast<wchar_t *>(manager_g->matcher->finddrp(bffw.Get()));
            if(!s)return;
            WStringShort buf;
            buf.sprintf(L"%ws\\%s",Settings.drp_dir,s);
            Log.print_con("Old file: %S\n",buf.Get());
            _wremove(buf.Get());
            return;
        }
        s++;
    }
}

void UpdaterImp::checkUpdates()
{
    // called from main and scripts
    #ifdef USE_TORRENT
    Timers.start(time_chkupdate);
    SetActiveTorrent(activetorrent);
    Timers.stop(time_chkupdate);

    if(Settings.flags&FLAG_AUTOUPDATE)
    {
        Settings.flags&=~FLAG_AUTOUPDATE;
        WelcomeDownloadAll();
    }
    #endif // USE_TORRENT
}

void UpdaterImp::ShowProgress(wchar_t *buf)
{
    // this is called from Manager::drawitem in manager.cpp
    // to fill in the buf with the current torrent data to display
    // updates the text on the updates SLOT thing

    #ifdef USE_TORRENT

    if(!hSession)
        return;
    if(!Torrents[activetorrent].handle.is_valid())
        return;

    wchar_t num1[BUFLEN],num2[BUFLEN],num3[BUFLEN],num4[BUFLEN];

    torrent_status st=Torrents[activetorrent].handle.status();
    int perc=st.progress*100;

    format_size(num1,st.total_wanted_done,0);
    format_size(num2,st.total_wanted,0);
    format_size(num3,st.upload_rate,1);
    format_size(num4,st.total_upload,0);


    // this is the special seed mode of the drivers directory
    if(st.upload_mode&&!(st.state==libtorrent::torrent_status::state_t::checking_files))
        wsprintf(buf,STR(STR_DWN_SEEDING),num4,num3);
    else if(IsMovingFiles)
        wsprintf(buf,STR(STR_TR_ST8));
    else if(IsFlushing)
        wsprintf(buf,STR(STR_DWN_CLOSING));
// deprecated
//        else if(st.state==torrent_status::queued_for_checking)
//            wsprintf(buf,STR(STR_TR_ST0));
    else if(st.state==torrent_status::checking_files)
        wsprintf(buf,STR(STR_UPD_CHECKINGFILES),num1,num2,perc);
    else if(st.state==torrent_status::downloading_metadata)
        wsprintf(buf,STR(STR_TR_ST2));
    else if(st.state==torrent_status::downloading)
        wsprintf(buf,STR(STR_UPD_PROGRES),num1,num2,perc);
    else if((st.state==torrent_status::finished)&&IsSeedingDrivers())
        wsprintf(buf,STR(STR_DWN_SEEDING),num4,num3);
    else if(st.state==torrent_status::finished)
        wsprintf(buf,STR(STR_TR_ST4));
    else if(st.state==torrent_status::seeding)
        wsprintf(buf,STR(STR_DWN_SEEDING),num4,num3);
// deprecated
//        else if(st.state==torrent_status::allocating)
//            wsprintf(buf,STR(STR_TR_ST6));
    else if(st.state==torrent_status::checking_resume_data)
        wsprintf(buf,STR(STR_TR_ST7));

    #else
    UNREFERENCED_PARAMETER(buf);
    #endif // USE_TORRENT
}

UpdaterImp::UpdaterImp()
{
    // constructor

    TorrentStartTime=0;
    AverageSpeed=0;
    ThreadManager_exitflag=THREAD_STATUS_WAITING;

    // synchronization event with manual reset
    ThreadManager_event=CreateEventWr(true);

    // used in the scripts
    installupdate_exitflag=0;
    installupdate_event=CreateEventWr(true);

    // create the session object
    StartTorrentSession();

    // download and create the torrents
    LoadTorrents();

    thandle_download=CreateThread();
    thandle_download->start(&thread_download,nullptr);
}

UpdaterImp::~UpdaterImp()
{
    // destructor
    #ifdef USE_TORRENT
    if(hSession)
    {
        Updater->StopTorrent();
        hSession->abort();
    }
    #endif // USE_TORRENT

    if(thandle_download)
    {

        ThreadManager_exitflag=THREAD_STATUS_ABORT;
        ThreadManager_event->wait(5000);
        ThreadManager_event->reset();
        // the thread refuses to join
        //thandle_download->join();
        //delete thandle_download;
        //delete downloadmangar_event;
    }
}


/*
 *
 *                     MAIN TORRENT THREAD
 *
*/

unsigned int __stdcall UpdaterImp::thread_download(void *arg)
{
    // this is the thread that handles the torrent session
    // it basically reacts to whatever the torrent session is up to
    // it is created in the UpdaterImp constructor
    // and destroyed in the UpdaterImp destructor

    #ifdef USE_TORRENT
    std::vector<lt::alert*> alerts;
    #endif // USE_TORRENT

	UNREFERENCED_PARAMETER(arg);
    // -verbose:4096
    Log.print_debug("{thread_download\n");

    #ifdef USE_TORRENT
    UpdaterImp *Updater1=dynamic_cast<UpdaterImp *>(Updater);
    #endif // USE_TORRENT


    //
    // main thread loop
    //

    Log.print_con("Torrent thread started\n");
    while(true)
    {

        // let the torrent do it's thing
        ThreadManager_event->wait(2000);

        // i've been told to quit the thread
        if(ThreadManager_exitflag==THREAD_STATUS_ABORT)
        {
            // notify the destructor when i finally get around to exiting
            ThreadManager_event->raise();
            break;
        }

        #ifdef USE_TORRENT

        // -autoupdate command line parameter
        if(Settings.flags&FLAG_AUTOUPDATE)
            Updater1->WelcomeDownloadAll();


        if(hSession)
        {
            // tell torrent to post_torrent_stats
            hSession->post_torrent_updates();
            if(hSession->wait_for_alert(std::chrono::seconds(2)))
            {
                // get a list of queued up alerts
                hSession->pop_alerts(&alerts);
                // iterate the returned alerts
                for (libtorrent::alert const* a : alerts)
                {
                    // update the stats msg
                    UpdateDialog.UpdateTorrentAlert(a->message());
                    UpdateDialog.SetControls();
                    std::string s=a->message();
                    if( (s!="state updates for 0 torrents") &&
                        (s.find("partfile_read")==std::string::npos) )
                        Log.print_torr("Torrent Alert: %s\n",s.c_str());


                    //
                    // Process specific alerts using alert_cast
                    //

                    // state update
                    if (lt::alert_cast<lt::state_update_alert>(a))
                    {
                        UpdateDialog.UpdateTorrentStats();
                        // update the progress without repopulating the listview
                        UpdateDialog.UpdateProgress();
                    }

                    else if (lt::alert_cast<lt::add_torrent_alert>(a))
                    {
                        UpdateDialog.UpdateTorrentStats();
                        // update the progress without repopulating the listview
                        UpdateDialog.UpdateProgress();
                    }


                    // error
                    else if(lt::alert_cast<lt::torrent_error_alert>(a))
                    {
                        if(auto* tc=lt::alert_cast<lt::torrent_error_alert>(a))
                        {
                            // i only want to react to the active torrent
                            int tn=Updater1->GetTorrentNumFromAlert(tc);
                            if(tn==activetorrent)
                            {
                                UpdateDialog.UpdateTorrentStats();
                            }
                        }
                    }


                    // torrent removed - hopefully this only comes from StopSpecialShare
                    else if(lt::alert_cast<lt::torrent_removed_alert>(a))
                    {
                        if(auto* tc=lt::alert_cast<lt::torrent_removed_alert>(a))
                        {
                            // i only want to react to the active torrent
                            int tn=Updater1->GetTorrentNumFromAlert(tc);
                            if(tn==activetorrent)
                            {
                                // reload the torrent from the file
                                Updater1->CreateTorrentFromFile(Torrents[0].FileName, Torrents[0].SavePath, Torrents[0].handle,false);
                                // let the torrent catch up
                                ThreadManager_event->wait(2000);
                                UpdateDialog.Populate(true);
                                UpdateDialog.SetControls();
                                IsEndingShareMode=false;
                            }
                        }
                    }

                    // torrent paused
                    else if(lt::alert_cast<lt::torrent_paused_alert>(a))
                    {
                        if(auto* tc=lt::alert_cast<lt::torrent_paused_alert>(a))
                        {
                            // i only want to react to the active torrent
                            int tn=Updater1->GetTorrentNumFromAlert(tc);
                            if(tn==activetorrent)
                            {
                                Updater1->ProcessFinishedTorrent();
                                ThreadManager_exitflag=THREAD_STATUS_WAITING;
                            }
                        }
                    }


                    // torrent finished
                    // might be finished checking files
                    else if(lt::alert_cast<lt::torrent_finished_alert>(a))
                    {
                        if(auto* tc=lt::alert_cast<lt::torrent_finished_alert>(a))
                        {
                            // i only want to react to the active torrent
                            int tn=Updater1->GetTorrentNumFromAlert(tc);
                            if(tn==activetorrent)
                            {
                                UpdateDialog.UpdatePromptMessage();
                                if(tn==0)
                                    Updater1->FlushTorrentCache(tn);
                                UpdateDialog.UpdateTorrentStats();
                            }
                        }
                    }

                    // cache flushed
                    else if(lt::alert_cast<lt::cache_flushed_alert>(a))
                    {
                        if(auto* tc=lt::alert_cast<lt::cache_flushed_alert>(a))
                        {
                            // i only want to react to the active torrent
                            int tn=Updater1->GetTorrentNumFromAlert(tc);
                            if(tn==activetorrent)
                            {
                                if(IsFlushing)
                                {
                                    // if checkbox is checked then leave the torrent active
                                    if(Settings.flags&FLAG_KEEPSEEDING)
                                        Torrents[activetorrent].handle.resume();
                                    else
                                        Torrents[activetorrent].handle.pause();
                                }
                                IsFlushing=false;
                            }
                        }
                    }


                }

            }

        }
        #endif // USE_TORRENT

    } // thread loop - keep looping until program ends


    Log.print_con("Torrent thread ended\n");
    return 0;
}

void UpdaterImp::pause()
{
    StopTorrent();
    ThreadManager_exitflag=THREAD_STATUS_WAITING;
    ThreadManager_event->raise();
}

bool UpdaterImp::IsUpdateCompleted()
{
    // called by install.cpp
    return finishedupdating;
}

int  UpdaterImp::Populate(bool reload)
{
    // called by manager.cpp : Populate(0)
    // if i've been moving files then force a reload
    reload=reload|IsMovingFiles;
    int ret=UpdateDialog.Populate(reload);
    IsMovingFiles=false;
    UpdateDialog.UpdatePromptMessage();
    UpdateDialog.SetControls();
    System.ProcessMessages();
    return ret;
}

void UpdaterImp::OpenDialog(int automode)
{
    UpdateDialog.OpenDialog(automode);
}



/*
    multi torrent

*/

void UpdaterImp::StartTorrentSession()
{
    #ifdef USE_TORRENT

    if(!hSession)
    {
        // this is the new way of setting libtorrent settings
        // any errors return via alerts
        char listen[BUFLEN];
        wsprintfA(listen,"0.0.0.0:%d,[::]:%d",torrentport,torrentport);
        libtorrent::settings_pack pack;
        pack.set_str(libtorrent::settings_pack::user_agent,"Snappy Driver Installer Origin " VER_VERSION_STR2 );
        pack.set_bool(libtorrent::settings_pack::always_send_user_agent,true);
        pack.set_bool(libtorrent::settings_pack::anonymous_mode,false);
        pack.set_int(libtorrent::settings_pack::choking_algorithm, libtorrent::settings_pack::rate_choker_initial_threshold);
        pack.set_bool(libtorrent::settings_pack::volatile_read_cache,false);
        pack.set_int(libtorrent::settings_pack::outgoing_port, outgoingport_min);
        pack.set_int(libtorrent::settings_pack::num_outgoing_ports, outgoingport_max-outgoingport_min+1);
        pack.set_str(libtorrent::settings_pack::listen_interfaces,listen);
        pack.set_bool(libtorrent::settings_pack::enable_lsd,true);
        pack.set_bool(libtorrent::settings_pack::enable_natpmp,true);
        pack.set_bool(libtorrent::settings_pack::enable_dht, true);
        pack.set_str(libtorrent::settings_pack::dht_bootstrap_nodes, "router.bittorrent.com:6881,router.utorrent.com:6881,router.bitcomet.com:6881");
        pack.set_int(libtorrent::settings_pack::alert_mask, libtorrent::alert_category::error |
                                                            //libtorrent::alert_category::tracker |
                                                            libtorrent::alert_category::ip_block |
                                                            //libtorrent::alert_category::dht |
                                                            libtorrent::alert_category::storage |
                                                            libtorrent::alert_category::status);
        hSession=new libtorrent::session(pack,session::add_default_plugins);

        // new settings can be applied to an existing session by using:
        // hSession->apply_settings(pack);

        Log.print_con("Torrent session started\n");
        Log.print_con("Listen port: %d (%s)\nDownload limit: %dKb\nUpload limit: %dKb\n",
                torrentport,hSession->is_listening()?"connected":"disconnected",
                downlimit,uplimit);
        if(outgoingport_min)
        {
            Log.print_con("Min outgoing port: %d\n",outgoingport_min);
            Log.print_con("Max outgoing port: %d\n",outgoingport_max);
        }
    }
    #endif // USE_TORRENT
}


bool UpdaterImp::IsSeedingDrivers()
{
    // seeding can be when the status is 'seeding'
    // or when status is 'finished' and the torrent is still running

    #ifdef USE_TORRENT

    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return false;

    lt::torrent_status status=CurrentTorrent.status();

    return status.state==libtorrent::torrent_status::state_t::seeding ||
           (status.state==libtorrent::torrent_status::state_t::finished &&
            !IsPaused());
    #else
    return false;
    #endif // USE_TORRENT
}

void UpdaterImp::SetLimits()
{
    #ifdef USE_TORRENT

    if(!hSession)
        return;

    for(int i=0;i<2;i++)
    {
        if(Torrents[i].handle.is_valid())
        {
            Torrents[i].handle.set_download_limit(downlimit*1024);
            Torrents[i].handle.set_upload_limit(uplimit*1024);
            if(connections)
                Torrents[i].handle.set_max_connections(connections);
        }
    }

    #endif // USE_TORRENT
}


// Helper to read the file into a buffer
std::vector<char> inline load_file(char const* filename) {
    std::ifstream in(filename, std::ios::binary);
    in.unsetf(std::ios::skipws);
    return std::vector<char>(std::istreambuf_iterator<char>(in), std::istreambuf_iterator<char>());
}

void UpdaterImp::LoadTorrents()
{
    #ifdef USE_TORRENT

    if(!hSession)
        return;

    std::vector<torrent_handle> torrents;

    // create the .torrent storage directory
    CreateDirectoryA(".\\torrent",nullptr);

    // remove any torrents in the session
    torrents=hSession->get_torrents();
    for (const auto& h : torrents)
        hSession->remove_torrent(h);

    // for all torrents in the array
    // *** back to just the first torrent for now
    for(int i=0;i<1;i++)
    {

        // create the torrent save directory
        CreateDirectoryA(Torrents[i].SavePath.c_str(),nullptr);

        // check save directory is writable
        if(!System.canWriteDirectory(Torrents[i].SavePath))
        {
            Log.print_err("Warning: save path is not writable (%s)\n",Torrents[i].SavePath);
            continue;
        }

        // delete the old .torrent
        DeleteFileA(Torrents[i].FileName.c_str());

        // the getDateTime is to ensure the web cache is not used
        std::string url=Torrents[i].URL+"?cb="+System.getDateTimeStr();

        // download the .torrent file from the web site
        URLDownloadToFileA(nullptr, url.c_str(), Torrents[i].FileName.c_str(), 0, nullptr);
        if(!System.FileExists(Torrents[i].FileName))
        {
            Log.print_err("ERROR: Failed to retrieve torrent (%S)", Torrents[i].URL);
            continue;
        }

        // create the torrent object from the .torrent file and add to the session &hTorrent
        CreateTorrentFromFile(Torrents[i].FileName, Torrents[i].SavePath, Torrents[i].handle,false);
    }

    // set user config speed limits
    SetLimits();
    #endif // USE_TORRENT
}

#ifdef USE_TORRENT
int UpdaterImp::CreateTorrentFromFile(std::string FileName, std::string SavePath, libtorrent::torrent_handle &handle, bool SeedMode)
{
    #ifdef USE_TORRENT

    libtorrent::add_torrent_params params;

    char cFileName[BUFSIZ];
    wsprintfA(cFileName,"%s",FileName.c_str());

    char cSavePath[BUFSIZ];
    wsprintfA(cSavePath,"%s",SavePath.c_str());

    // loading the .torrent file
    std::vector<char> buffer = load_file(cFileName);
    // parse the file contents
    libtorrent::error_code ec;
    libtorrent::bdecode_node node=libtorrent::bdecode(buffer, ec);
    // add the parsed data to the torrent_info
    params.ti = std::make_shared<libtorrent::torrent_info>(node, ec);

    // download save path
    params.save_path=cSavePath;



    // libtorrent::torrent_flags::auto_managed
    // libtorrent::torrent_flags::seed_mode
    // params.flags|=libtorrent::torrent_flags::stop_when_ready;

    params.flags=libtorrent::torrent_flags::paused;
    // seed mode is for strict upload only mode
    if(SeedMode)
    {
        params.flags|=libtorrent::torrent_flags::upload_mode;
        params.flags|=libtorrent::torrent_flags::seed_mode;
    }

    // auto_managed is on by default - i must turn it off
    params.flags &= ~libtorrent::torrent_flags::auto_managed;

    // add the new torrent to the existing session
    handle=hSession->add_torrent(params,ec);
    if(ec)
    {
        Log.print_err("ERROR: Failed to add torrent: %s\n",ec.message().c_str());
        return 0;
    }

    // check the torrent is valid
    if(!handle.is_valid())
    {
        if(emptydrp) manager_g->itembar_settext(SLOT_NODRIVERS,1);
        Log.print_err("ERROR: Torrent is not valid\n");
        Log.print_con("FAILED\n");
        return 0;
    }

    //handle.pause();
    std::shared_ptr<const libtorrent::torrent_info> info=handle.torrent_file();

    // set everything to 'no download' in anticipation of user
    // opening dialog and selecting which files to download
    // 0==no download, 1=default, 2-7=priority (higher=more priority)
    for(int j=0;j<info->num_files();j++)
        handle.file_priority(j,0);

    return 1;
    #endif // USE_TORRENT
}
#endif

void UpdaterImp::StartInstallDownload(std::vector<std::wstring> filenames)
{
    // called by install.cpp
    // give me a list of driver packs to download

    // default return
    finishedupdating=true;

    #ifdef USE_TORRENT

    if(!hSession)
        return;

    // this is coming from the click-and-download-and-install
    // function on the main screen
    // by virtue of using the online indexes "_P_"
    // we know the files are in the first torrent
    Updater->SetActiveTorrent(0);

    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;

    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();
    libtorrent::torrent_status status=CurrentTorrent.status(status_flags_t::all());
    std::string f,fp;

    // reset all file priorities
    for(int j=0;j<info->num_files();j++)
        CurrentTorrent.file_priority(j,0);

    int foundcount=0;

    // iterate the filenames given to me
    int filecount=filenames.size();
    if(filecount)
    {
        for(std::wstring filename : filenames)
        {
            f=CopyWcharToUtf8String(filename.c_str());
            // find this file name in the torrent
            for(int i=0;i<info->num_files();i++)
            {
                fp=fs.file_path(i);
                if(StrStrIA(fp.c_str(),f.c_str()))
                {
                    // found the file - set the priority to 1
                    CurrentTorrent.file_priority(i,1);
                    Log.print_con("Torrent %d: req %s\n",activetorrent,f.c_str());
                    foundcount++;
                }
            }
        }

        // see if i found the files in the torrent
        if(!foundcount)
        {
            Log.print_con("Torrent %d: requested files not found\n",activetorrent);
        }
        else
        {
            CurrentTorrent.resume();
            TorrentStartTime=System.GetTickCountWr();
            AverageSpeed=0;
            finishedupdating=0;
            InstallDownloadRunning=true;
        }
    }
    #else
    UNREFERENCED_PARAMETER(filenames);
    #endif // USE_TORRENT
}

void UpdaterImp::EndInstallDownload()
{
    // flag to indicate install download in progress
    // in ProcessFinished don't run the startup code
    // at the end if the flag is set
    // run the startup code here instead

    if(InstallDownloadRunning)
    {
        // start everything up
        finishedupdating=true;
        monitor_pause=0;
        IsMovingFiles=false;

        // this should trigger the populate
        // relies on the filemon in system.cpp working
        if((invaidate_set&INVALIDATE_INDEXES)==0)
            invalidate(INVALIDATE_INDEXES|INVALIDATE_MANAGER);

        InstallDownloadRunning=false;
    }
}

void UpdaterImp::StartSpecialShare()
{
    #ifdef USE_TORRENT

    if(!hSession)
        return;

    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();

    Log.print_con("Torrent: start share mode");
    IsStartingShareMode=true;
    UpdateDialog.UpdatePromptMessage();
    UpdateDialog.SetControls();
    System.ProcessMessages();

    // to enable strict seed mode i need to remove the torrent from the session
    // add the flags then re-add it to the session
    hSession->remove_torrent(CurrentTorrent);
    CreateTorrentFromFile(Torrents[0].FileName, ".\\", Torrents[0].handle,true);

    // let the torrent catch up
    ThreadManager_event->wait(2000);

    // get the new handle
    CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;

    // the torrent contains the wrong directory structure for this purpose
    // so the plan is to reset the path of the driver entries
    // and disable non-driver entries

    // the file will be renamed to just the file name (ie remove drivers directory)
    // the save path is set to drivers above
    // the result will be .\drivers\dp_xxxx.7z

    wchar_t buf[BUFLEN];
    int cnt=0;

    // iterate the torrent files
    for(int i=0;i<info->num_files();i++)
    {
        std::string fp=fs.file_path(i);

        if(StrStrIA(fp.c_str(),"drivers\\"))
        {
            // relocate the entry to the driver directory
            std::string filename{fs.file_name(i)};
            std::wstring ws1=utf8_to_wstring(filename);
            wsprintf(buf,L"%ws\\%ws",Settings.drp_dir,ws1.c_str());
            CurrentTorrent.rename_file(i, buf);
            // if file exists in the drp_dir directory than it can be shared
            if(System.FileExists(buf))
            {
                CurrentTorrent.file_priority(i,1);
                cnt++;
            }
            else
                CurrentTorrent.file_priority(i,0);
        }
        else
            // disable non driver entries
            CurrentTorrent.file_priority(i,0);
    }


    if(cnt==0)
    {
        Log.print_con(", nothing to share\n");
        StopSpecialShare();
        IsStartingShareMode=false;
        return;
    }

    // let the torrent catch up
    ThreadManager_event->wait(2000);

    Log.print_con(", %d drivers to share\n",cnt);

    // resume first then set seeding mode
    CurrentTorrent.set_upload_mode(true);
    CurrentTorrent.resume();

    UpdateDialog.SetControls();
    UpdateDialog.Populate(true);
    IsStartingShareMode=false;

    #endif // USE_TORRENT
}

void UpdaterImp::StopSpecialShare()
{
    #ifdef USE_TORRENT

    if(!hSession)
        return;

    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;

    Log.print_con("Torrent: stop share mode\n");
    IsEndingShareMode=true;
    UpdateDialog.UpdatePromptMessage();
    UpdateDialog.SetControls();
    System.ProcessMessages();

    // remove the torrent - this will trigger an alert
    // which will reset the normal torrent
    hSession->remove_torrent(CurrentTorrent);

    #endif // USE_TORRENT
}

void UpdaterImp::StartTorrent()
{
    #ifdef USE_TORRENT

    if(!hSession)
        return;

    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;

    finishedupdating=0;
    IsInitializing=true;

    // how many files to download
    int CheckCount=UpdateDialog.setPriorities();

    // a shortcut - if none checked then check all
    if(!CheckCount)
        UpdateDialog.SetAllDriverCheckboxes();

    // how many files to download
    CheckCount=UpdateDialog.setPriorities();

    if(CheckCount>0)
    {
        Log.print_con("Torrent %d: resuming, %d files to get...\n",activetorrent,CheckCount);
        CurrentTorrent.resume();
        UpdateDialog.UpdateTorrentStats();
        UpdateDialog.UpdatePromptMessage();
        UpdateDialog.SetControls();
        System.ProcessMessages();
        TorrentStartTime=System.GetTickCountWr();

        AverageSpeed=0;
    }

    IsInitializing=false;
    #endif // USE_TORRENT
}

void UpdaterImp::StopTorrent()
{
    // called from UpdateDialog stop button
    // and main form download slot thing

    #ifdef USE_TORRENT

    if(!hSession)
        return;
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;

    if(CurrentTorrent.is_valid())
    {
        // switch off seeding
        Settings.flags&=~FLAG_KEEPSEEDING;
        UpdateDialog.SetCheckbox(ID_UPD_KEEPSEEDING,0);
        // shut off the torrent
        CurrentTorrent.pause();
        // this will trigger an alert when the flush is complete
        FlushTorrentCache(activetorrent);
    }
    UpdateDialog.SetControls();
    #endif // USE_TORRENT
}

void UpdaterImp::SetActiveTorrent(const int torrent)
{
    #ifdef USE_TORRENT

    if(!hSession)
        return;

    if(!Torrents[activetorrent].handle.is_valid())
        return;

    if(torrent>=0&&torrent<=1)
    {
        // pause the current active torrent
        if(!(Torrents[activetorrent].handle.status().flags & libtorrent::torrent_flags::paused))
        {
            Torrents[activetorrent].handle.pause();
            Torrents[activetorrent].handle.flush_cache();
        }

        // set the new active torrent
        activetorrent=torrent;

        // Populate list and return available updates
        int ret=UpdateDialog.Populate(true);

        // check for SDIO exe in the torrent
        if(UpdateDialog.TorrentRevision==0)
            Log.print_con("Latest Version: Not found.\n");
        else if(UpdateDialog.TorrentRevision<=UpdateDialog.LocalRevision)
            Log.print_con("Latest Version: R%d. Up to date.\n",UpdateDialog.TorrentRevision);
        else
            Log.print_con("Latest Version: R%d.\n",UpdateDialog.TorrentRevision);

        Log.print_con("Updated driver packs available: %d\n",ret&0xFF);

        return;
    }
    #endif // USE_TORRENT
}

void UpdaterImp::ProcessFinishedTorrent()
{
    // called after torrent paused alert

    #ifdef USE_TORRENT

    if(!Torrents[activetorrent].handle.is_valid())
        return;

    // stop everything
    // pause the file system monitor
    monitor_pause=1;
    IsMovingFiles=true;

    // force update on the gui
    UpdateDialog.UpdateTorrentStats();
    UpdateDialog.UpdatePromptMessage();
    System.ProcessMessages();

    TorrentStartTime=0;

    // Move files
    int MoveCount=MoveNewFiles();



    RemoveRedundantDriverPacks();

    // Execute user cmd
    if(*Settings.finish_upd)
    {
        WStringShort buf;
        buf.sprintf(L" /c %s",Settings.finish_upd);
        System.run_command(L"cmd",buf.Get(),SW_HIDE,0);
    }

/* this works in case i need it elsewhere for print_con
    wchar_t buf[BUFLEN];
    std::wstring ws1,ws2;
    ws1=STR(STR_TR_ST4);
    wsprintf(buf,ws1.c_str());
*/

    // finished
    Log.print_con("Torrent %d: finished\n", activetorrent);

    // auto exit on completion
    if((Settings.flags&FLAG_AUTOCLOSE)&&!(Settings.flags&FLAG_AUTOINSTALL))
        PostMessage(MainWindow.hMain,WM_CLOSE,0,0);

    finishedupdating=true;

    if(!InstallDownloadRunning)
    {
        // start everything up
        monitor_pause=0;

        // defer this until manager triggers me, Updater->Populate(0)
        //IsMovingFiles=false;

        // i can't refresh the list view yet
        // because manager hasn't refreshed itself
        // this should trigger the populate
        // relies on the filemon in system.cpp working
        if(MoveCount)
            invalidate(INVALIDATE_INDEXES|INVALIDATE_MANAGER);
        else
        {
            IsMovingFiles=false;
            UpdateDialog.UpdatePromptMessage();
            UpdateDialog.SetControls();
            System.ProcessMessages();
        }
    }

    manager_g->itembar_settext(SLOT_DOWNLOAD,1,nullptr,-1,-1,0);

    // put it back to sleep
    ThreadManager_exitflag=THREAD_STATUS_WAITING;
    #endif // USE_TORRENT
}
#ifdef USE_TORRENT
int UpdaterImp::GetTorrentNumFromAlert(const libtorrent::torrent_alert* alert)
{
    // this figures out which of my torrents the alert refers to

    int ret=-1;

    #ifdef USE_TORRENT

    if(alert->handle==Torrents[0].handle)
        ret=0;
    else if(alert->handle==Torrents[1].handle)
        ret=1;

    #endif // USE_TORRENT

    return ret;
}
#endif // USE_TORRENT

void UpdaterImp::FlushTorrentCache(const int torrent_num)
{
    // Flush the torrent disk cache
    // this will trigger an alert when the flush is complete
    // which will call pause which will trigger an alert
    // which will call ProcessFinishedTorrent
    // etc ....

    Log.print_con("Torrent %d: flushing cache...\n",torrent_num);

    #ifdef USE_TORRENT
    if(Torrents[torrent_num].handle.is_valid())
    {
        IsFlushing=true;
        UpdateDialog.UpdatePromptMessage();
        System.ProcessMessages();
        Torrents[torrent_num].handle.flush_cache();
    }
    #endif // USE_TORRENT
}

bool UpdaterImp::IsPaused()
{
    // this is the rather convoluted approach libtorrent
    // takes to deciding when the torrent is paused
    #ifdef USE_TORRENT

    if(hSession&&Torrents[activetorrent].handle.is_valid())
    {
        libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
        bool IsPaused = (CurrentTorrent.status().flags & (lt::torrent_flags::auto_managed | lt::torrent_flags::paused)) == lt::torrent_flags::paused ? true : false;
        return IsPaused;
    }

    #endif // USE_TORRENT
    return false;
}

int UpdaterImp::SetFilePriorities()
{
    #ifdef USE_TORRENT

    if(!Torrents[activetorrent].handle.is_valid())
        return 0;

    return UpdateDialog.setPriorities();
    #else
    return 0;
    #endif // USE_TORRENT
}

int UpdaterImp::MoveNewFiles()
{
    // move completed files to drp_dir, index_dir, base

    int CompleteFiles=0;
    int IncompleteFiles=0;

    std::string fp;
    std::wstring SourceFile;
    std::wstring DestFile;
    std::wstring IndexDir={Settings.index_dir};
    std::wstring DrpDir={Settings.drp_dir};

    #ifdef USE_TORRENT
    int i;

    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return 0;

    // get torrent info
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();

    // get file progress
    std::vector<std::int64_t> file_progress;
    CurrentTorrent.file_progress(file_progress,false);


    //
    // i'm going to iterate the torrent files 3 times cos it's easier
    // and don't trash my source files while testing!!  :-|
    //


    //
    // application files - all files that are not drivers and not indexes are "Application"
    //
    int64_t appgot=0; int64_t appsize=0;
    for(i=0;i<info->num_files();i++)
    {
        fp=fs.file_path(i);
        if( (to_lower(fp).find("sdio_update\\indexes")==std::string::npos) &&
            (to_lower(fp).find("sdio_update\\drivers")==std::string::npos) &&
            (CurrentTorrent.file_priority(i)==2) )
        {
            appsize+=fs.file_size(i);
            appgot+=file_progress[i];
        }
    }

    // if i checked 'em and i got 'em then process application files
    if(appsize>0)
    {
        if(appgot==appsize)
        {
            for(i=0;i<info->num_files();i++)
            {
                fp=fs.file_path(i);
                if( (to_lower(fp).find("sdio_update\\indexes")==std::string::npos) &&
                    (to_lower(fp).find("sdio_update\\drivers")==std::string::npos) &&
                    (CurrentTorrent.file_priority(i)==2)  )
                {
                    SourceFile=utf8_to_wstring("update\\"+fp);
                    DestFile=utf8_to_wstring(fp.substr(fp.find("\\")+1));
                    Log.print_con("Move: %S\n",DestFile.c_str());
                    if(!System.FileExists(SourceFile.c_str()))
                        Log.print_err("File not found: %S\n",SourceFile.c_str());
                    else if(!System.MoveFile(SourceFile,DestFile))
                       Log.print_err("Move error: %S\n",SourceFile.c_str());
                }
            }
            CompleteFiles++;
        }
        else
            IncompleteFiles++;
    }




    //
    // index files - all files in the indexes directory are "Indexes"
    //
    // first make sure I checked them and I've got them all
    int64_t ndxgot=0; int64_t ndxsize=0;
    for(i=0;i<info->num_files();i++)
    {
        fp=fs.file_path(i);
        if( (to_lower(fp).find("sdio_update\\indexes")!=std::string::npos) &&
            (CurrentTorrent.file_priority(i)==2) )
          {
              ndxsize+=fs.file_size(i);
              ndxgot+=file_progress[i];
          }
    }

    // if i checked em and i got em then process index files
    if(ndxsize>0)
    {
        if(ndxgot==ndxsize)
        {
            // delete old "_P_" indexes
            System.DeleteFilesWithWildcard(IndexDir, L"_P_*.bin");
            for(i=0;i<info->num_files();i++)
            {
                fp=fs.file_path(i);
                if( (to_lower(fp).find("sdio_update\\indexes")!=std::string::npos) &&
                    (CurrentTorrent.file_priority(i)==2) )
                {
                    // have to rename index file while moving
                    SourceFile=utf8_to_wstring("update\\"+fp);
                    DestFile=utf8_to_wstring(fp.substr(fp.rfind("\\")+1));
                    DestFile=IndexDir + L"\\_" + DestFile.substr(1);
                    Log.print_con("Move: %S\n",DestFile.c_str());
                    if(!System.FileExists(SourceFile.c_str()))
                        Log.print_err("File not found: %S\n",SourceFile.c_str());
                    else if(!System.MoveFile(SourceFile,DestFile))
                        Log.print_err("Move error: %S\n",SourceFile.c_str());
                }
            }
            CompleteFiles++;
        }
        else
            IncompleteFiles++;
    }





    //
    // driver files - all files in the drivers directory are the individual driver files
    //
    for(i=0;i<info->num_files();i++)
    {
        fp=fs.file_path(i);
        if( (to_lower(fp).find("sdio_update\\drivers")!=std::string::npos) &&
            (CurrentTorrent.file_priority(i)==1) )
        {
            if(file_progress[i]==fs.file_size(i))
            {
                // process driver file
                SourceFile=utf8_to_wstring("update\\"+fp);
                DestFile=DrpDir + L"\\" + utf8_to_wstring(fp.substr(fp.rfind("\\")+1));
                // remove old driver pack first
                wchar_t ws[BUFLEN];
                wsprintf(ws,L"%ws",DestFile.c_str());
                RemoveOldDriverpacks(ws+8);
                // move new driver pack
                Log.print_con("Move: %S\n",DestFile.c_str());
                if(!System.FileExists(SourceFile.c_str()))
                    Log.print_err("File not found: %S\n",SourceFile.c_str());
                else if(!System.MoveFile(SourceFile,DestFile))
                    Log.print_err("Move error: %S\n",SourceFile.c_str());
                CompleteFiles++;
            }
            else
                IncompleteFiles++;
        }
    }


/* *** old code

    // iterate the torrent files
    for(i=0;i<info->num_files();i++)
    {
        if(CurrentTorrent.file_priority(i))
        {
            std::string filenamefull=fs.file_path(i);
            // Skip autorun.inf and 2 batch files
            if(StrStrIA(filenamefull.c_str(),"autorun.inf")||StrStrIA(filenamefull.c_str(),".bat"))continue;


            fp=fs.file_path(i);


            // determine if file is completely downloaded
            bool FileIsComplete=file_progress[i]==fs.file_size(i);
            if(!FileIsComplete)
            {
                IncompleteFiles++;
                continue;
            }

            CompleteFiles++;

            // get the file name in the save path
            std::wstring wstr = utf8_to_wstring(Torrents[activetorrent].SavePath);
            wchar_t filenamefull_src[BUFLEN];
            std::string fp=fs.file_path(i);
            wsprintf(filenamefull_src,L"%s\\%S", wstr.c_str(),fp.c_str());

            // skip 0 byte files - something bad happened if this arises
            __int64 fileSize=System.FileSize(filenamefull_src);
            if(fileSize==0)
                continue;

            // Determine destination dirs
            wchar_t filenamefull_dst[BUFLEN];
            wsprintf(filenamefull_dst,L"%S",filenamefull.c_str());
            strsub(filenamefull_dst,L"indexes\\SDIO",Settings.index_dir);
            strsub(filenamefull_dst,L"drivers",Settings.drp_dir);
            strsub(filenamefull_dst,L"tools\\SDIO",Settings.data_dir);

            // Delete old driverpacks
            if(StrStrIA(filenamefull.c_str(),"drivers\\"))
                RemoveOldDriverpacks(filenamefull_dst+8);

            // Prepare "_" online indexes
            wchar_t *p=filenamefull_dst;
            if(p)
            {
                while(wcschr(p,L'\\'))p=wcschr(p,L'\\')+1;
                if(StrStrIW(filenamefull_src,L"indexes\\SDIO\\"))
                    *p=L'_';

                // Create dirs for the file
                WStringShort dirs;
                dirs.append(filenamefull_dst);
                p=dirs.GetV();
                while(wcschr(p,L'\\'))p=wcschr(p,L'\\')+1;
                if(p[-1]==L'\\')
                {
                    *--p=0;
                    mkdir_r(dirs.Get());
                }
            }

            // Delete old "_" online indexes if new are downloaded
            std::string s=WStringToString(filenamefull_src);
            if(CurrentTorrent.file_priority(i)&&StrStrIA(s.c_str(),"indexes\\SDIO"))
            {
                WStringShort buf;
                buf.sprintf(L"/c del %ws",filenamefull_dst);
                System.run_command(L"cmd",buf.Get(),SW_HIDE,1);
            }

            // can't move a file to a different drive
            // instead do a copy/delete

            // get current working drive
            wchar_t* buffer;
            int cwdDrive=-1;
            if ( (buffer = _wgetcwd(nullptr,BUFLEN) ) == nullptr)
                Log.print_con("_wgetcwd error");
            else
                cwdDrive = System.DriveNumber(buffer);

            // find the source  drive
            int srcDrive = System.DriveNumber(filenamefull_src);
            if (srcDrive==-1) srcDrive=cwdDrive;
            // Log.print_con("Src: %d %S\n",srcDrive,filenamefull_src);
            // find the destination drive
            int destDrive = System.DriveNumber(filenamefull_dst);
            if ( (wcscspn(filenamefull_dst,L"\\\\")!=0) && (destDrive==-1) ) destDrive=cwdDrive;
            //Log.print_con("Dst: %d %S\n",destDrive,filenamefull_dst);

            // if source and destination drive are the same then perform a move
            bool DoMove=(srcDrive==destDrive);

            if(DoMove)
            {
                // Move file
                Log.print_con("Move new file: %S\n",filenamefull_dst);
                if(!MoveFileEx(filenamefull_src,filenamefull_dst,MOVEFILE_REPLACE_EXISTING||
                                                                 MOVEFILE_COPY_ALLOWED||
                                                                 MOVEFILE_WRITE_THROUGH))
                                                                 {
                                                                    Log.print_syserr(GetLastError(),L"MoveFileEx()");
                                                                    DoMove=false;
                                                                 }
            }

            // if move fails or not same drive then perform a copy / delete
            if(!DoMove)
            {
                Log.print_con("Copy new file: %S\n",filenamefull_dst);
                if(!CopyFileExW(filenamefull_src, filenamefull_dst,nullptr,nullptr,nullptr,0))
                    Log.print_syserr(GetLastError(),L"CopyFileExW()");
                else if(System.FileExists(filenamefull_dst))
                    System.deletefile(filenamefull_src);
            }
        }
    }
*/

    #endif // USE_TORRENT

    Log.print_con("Torrent %d: %d complete files, %d incomplete files\n", activetorrent,CompleteFiles,IncompleteFiles);
    return CompleteFiles;
}


/*
 *
 *                     WELCOME DIALOG
 *
*/
#ifdef USE_TORRENT
void UpdaterImp::WelcomeDownloadAll()
{
    // called from the Welcome dialog

    #ifdef USE_TORRENT

    if(!hSession)
        return;
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;
    Settings.flags&=~FLAG_AUTOUPDATE;
    OpenDialog(1);
    #endif // USE_TORRENT
}

void UpdaterImp::WelcomeDownloadNetwork()
{
    // called from the Welcome dialog
    #ifdef USE_TORRENT

    if(!hSession)
        return;
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;
    OpenDialog(2);
    #endif // USE_TORRENT
}

void UpdaterImp::WelcomeDownloadIndexes()
{
    // called from the Welcome dialog

    #ifdef USE_TORRENT

    if(!hSession)
        return;
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;
    OpenDialog(3);
    #endif // USE_TORRENT
}


/*
 *
 *                     SCRIPTING
 *
*/

int UpdaterImp::scriptInitUpdates(int _torrentport)
{
    // the 'checkupdates' command

    Updater->torrentport=_torrentport;

    // the Updater constructor has already
    // set up the session and loaded the torrents
    // SetActiveTorrent has already been called


    #ifdef USE_TORRENT

    // get torrent info
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
    {
        Log.print_con("Torrent: download failed\n");
        return 1;
    }

    // get torrent info
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
//    const libtorrent::file_storage& fs = info->files();


    Settings.flags|=FLAG_UPDATESOK;
    Log.print_con("Torrent %d: initialized\n",activetorrent);
    #endif // USE_TORRENT

    return 0;
}

int UpdaterImp::scriptDownloadApp()
{
    if((Settings.flags&FLAG_UPDATESOK)==0)
    {
        Log.print_err("Error: get : Updates not initialised");
        return 1;
    }

    #ifdef USE_TORRENT

    // get torrent info
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return 1;

    // get torrent info
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();


    // select everything that's not indexes and drivers in the torrent
    for(int i=0;i<info->num_files();i++)
    {
        if(!(StrStrIA(fs.file_path(i).c_str(),"indexes\\"))&&
           !(StrStrIA(fs.file_path(i).c_str(),"drivers\\")))
            CurrentTorrent.file_priority(i,2);
        else
            CurrentTorrent.file_priority(i,0);
    }
    #endif // USE_TORRENT

    return scriptDoDownload();
}

int UpdaterImp::scriptDownloadIndexes()
{
    if((Settings.flags&FLAG_UPDATESOK)==0)
    {
        Log.print_err("Error: get : Updates not initialised");
        return 1;
    }

    #ifdef USE_TORRENT

    // get torrent info
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return 1;

    // get torrent info
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();


    // select indexes in the torrent
    for(int i=0;i<info->num_files();i++)
    {
        std::string file=fs.file_path(i);
        size_t p=file.find("indexes\\");
        if(p!=std::string::npos)
            CurrentTorrent.file_priority(i,2);
        else
            CurrentTorrent.file_priority(i,0);
    }
    #endif // USE_TORRENT

    return scriptDoDownload();
}

int UpdaterImp::scriptDownloadDrivers(std::wstring mode)
{

    if((Settings.flags&FLAG_UPDATESOK)==0)
    {
        Log.print_err("Error: get : Updates not initialised");
        return 1;
    }


    #ifdef USE_TORRENT

    // get torrent info
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return 1;

    // get torrent info
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();


    Log.print_debug("%d items selected\n",manager_g->selected());

    bool all=_wcsicmp(mode.c_str(),L"all")==0;
    bool missing=_wcsicmp(mode.c_str(),L"missing")==0;
    bool updates=_wcsicmp(mode.c_str(),L"updates")==0;
    bool selected=_wcsicmp(mode.c_str(),L"selected")==0;

    // set all active
    manager_g->filter(126);

    int updatecount=0;

    // iterate the torrent
    for(int i=0;i<info->num_files();i++)
    {
        // disable all files by default
        CurrentTorrent.file_priority(i,0);
        std::string file=fs.file_path(i);
        // look for driver entries
        size_t p=file.find("drivers\\");
        if(p!=std::string::npos)
        {
            file.erase(0,p+8);
            int curver=System.getcurver(file.c_str());
            int newver=System.getver(file.c_str());
            bool getfile=false;
            if(all)
                getfile=newver>curver;
            else if(missing)
                getfile=newver>curver&&!curver;
            else if(updates)
                getfile=newver>curver&&curver;
            else if (selected&&newver>curver)
            {
                std::wstring widestr = std::wstring(file.begin(), file.end());
                getfile=manager_g->isSelected(widestr.c_str()) &&
                        !manager_g->manager_drplive(widestr.c_str()); // 0 = yes
            }
            if(getfile)
            {
                Log.print_debug("Getting: %s\n", file.c_str());
                CurrentTorrent.file_priority(i,1);
                updatecount++;
            }
        }
    }

    if(!updatecount)
    {
        Log.print_con("Driver packs are up to date, nothing to do\n");
        return 0;
    }

    Log.print_con("Getting %d driver packs\n",updatecount);

    #else
    UNREFERENCED_PARAMETER(mode);

    #endif // USE_TORRENT

    return scriptDoDownload();
}

int UpdaterImp::scriptDownloadEverything()
{

    if((Settings.flags&FLAG_UPDATESOK)==0)
    {
        Log.print_err("Error: get : Updates not initialised");
        return 1;
    }

    #ifdef USE_TORRENT

    // get torrent info
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return 1;

    // get torrent info
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();



    // reset all files in the torrent
    for(int i=0;i<info->num_files();i++)
        CurrentTorrent.file_priority(i,0);

    // select everything that's not drivers
    for(int i=0;i<info->num_files();i++)
    {
        if(!(StrStrIA(fs.file_path(i).c_str(),"drivers\\")))
            CurrentTorrent.file_priority(i,2);
    }

    // missing and updated drivers
    for(int i=0;i<info->num_files();i++)
    {
        std::string file=to_lower(fs.file_path(i));
        // look for driver entries
        size_t p=file.find("drivers\\");
        if(p!=std::string::npos)
        {
            file.erase(0,p+8);
            int curver=System.getcurver(file.c_str());
            int newver=System.getver(file.c_str());
            // newer or missing
            if(newver>curver)
            {
                Log.print_debug("Getting: %s\n", file.c_str());
                CurrentTorrent.file_priority(i,1);
            }
        }
    }
    #endif // USE_TORRENT
    return scriptDoDownload();
}

void UpdaterImp::scriptRemaining()
{
    // get torrent info
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return;
    lt::torrent_status st=CurrentTorrent.status();
    int64_t remaining=0;
    std::wstring ws1;
    wchar_t buf[BUFLEN];
    wchar_t num2[BUFLEN];

        // elapsed
        if(TorrentStartTime>0)
        {

            if(st.download_rate)
            {
                AverageSpeed=static_cast<int>(SMOOTHING_FACTOR*st.download_rate+(1-SMOOTHING_FACTOR)*AverageSpeed);
                if(AverageSpeed)remaining=(st.total_wanted-st.total_wanted_done)/AverageSpeed*1000;
            }

            // remaining
            ws1=L""; ws1.append(wcslen(buf)+1,' ');
            Log.print_con("\r%S",ws1.c_str());
            ws1=STR(STR_UPD_STREAM_TIME);
            format_time(num2,remaining);
            wsprintf(buf,ws1.c_str(),num2);
            Log.print_con("\r%S",buf);
        }
}

int UpdaterImp::scriptDoDownload()
{
    std::wstring ws1;

    #ifdef USE_TORRENT

    // do i need to pause while torrent selection catches up??
    ThreadManager_event->wait(5000);

    // get torrent info
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return 1;

    // get torrent info
    lt::torrent_status st;
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();



    // how many files to get
    int count=0; int pr=0;
    for(int i=0;i<info->num_files();i++)
    {
        pr=CurrentTorrent.file_priority(i);
        if(pr)count++;
    }
    if(!count)return 1;


    Log.print_con("Torrent %d: resuming, %d files to get...\n",activetorrent,count);
    finishedupdating=0;
    CurrentTorrent.resume();
    TorrentStartTime=System.GetTickCountWr();
    AverageSpeed=0;


    while(!IsUpdateCompleted())
    {
        ThreadManager_event->wait(5000);
        scriptRemaining();
    }
    Log.print_con("\n");
    #endif // USE_TORRENT

    return 0;
}

int UpdaterImp::scriptInstall()
{
    std::wstring ws1;

    // check if anything selected
    if(manager_g->selected()==0)
    {
        Log.print_err("Error: install : Nothing selected.\n");
        return 1;
    }

    #ifdef USE_TORRENT

    // get torrent info
    libtorrent::torrent_handle CurrentTorrent=Torrents[activetorrent].handle;
    if(!CurrentTorrent.is_valid())
        return 1;

    // get torrent info
    lt::torrent_status st=CurrentTorrent.status();
    std::shared_ptr<const libtorrent::torrent_info> info=CurrentTorrent.torrent_file();
    const libtorrent::file_storage& fs = info->files();

    manager_g->itembar_setactive(SLOT_RESTORE_POINT,0);

    // i think this will do a download if required

    // switch off all torrent files by default
    if(Settings.flags&FLAG_UPDATESOK)
    {
        for(int i=0;i<fs.num_files();i++)
            CurrentTorrent.file_priority(i,0);
    }

    Log.print_con("Installing.");

    manager_g->install(INSTALLDRIVERS);
    installupdate_exitflag=0;

    while(installupdate_exitflag==0)
    {
        installupdate_event->wait(5000);
        scriptRemaining();
    }
    Log.print_con("\n");
    return (installupdate_exitflag==1?0:1); // 1=success


    #endif // USE_TORRENT
    return 1;
}
#endif // USE_TORRENT






