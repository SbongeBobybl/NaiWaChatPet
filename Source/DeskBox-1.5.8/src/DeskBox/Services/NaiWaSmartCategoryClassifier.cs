using DeskBox.Models;

namespace DeskBox.Services;

public sealed record NaiWaSmartClassification(
    List<AiOrganizeCategory> Categories,
    Dictionary<string, string> CategoryByPath,
    Dictionary<string, int> Counts);

/// <summary>
/// Name/extension based taxonomy for the NaiWa organizer. The model may refine
/// the names later, but this classifier always produces a useful 6–10 category
/// split offline so the preview visibly changes even without any network call.
/// </summary>
public static class NaiWaSmartCategoryClassifier
{
    public const int MinimumCategories = 6;
    public const int MaximumCategories = 10;

    private sealed record Bucket(
        string Id,
        string Name,
        int Order,
        HashSet<string> Extensions,
        string[] NameKeywords,
        string[] TokenKeywords);

    private sealed record SplitRule(
        string SourceId,
        Func<DesktopOrganizationFileSnapshot, string?> Match,
        string TargetId,
        string TargetName,
        int Order);

    private static readonly HashSet<string> AudioExtensions = Set(
        ".mp3",".wav",".flac",".aac",".m4a",".ogg",".wma",".opus",".aiff",".mid",".midi");
    private static readonly HashSet<string> VideoExtensions = Set(
        ".mp4",".mkv",".mov",".avi",".wmv",".webm",".m4v",".flv",".mpg",".mpeg",".ts",".rmvb");
    private static readonly HashSet<string> ArchiveExtensions = Set(
        ".zip",".7z",".rar",".tar",".gz",".bz2",".xz",".zst",".cab",".iso",".img");
    private static readonly HashSet<string> InstallerExtensions = Set(
        ".exe",".msi",".msix",".appx",".appxbundle",".msixbundle",".msp",".msu");

    private static readonly Bucket[] Buckets =
    [
        new("ai","AI与模型",10,Set(".gguf",".safetensors",".onnx",".ckpt"),[
            "大模型","模型下载","模型文件","智能助手","提示词","提示工程","对话记录","知识库"],["ai","gpt","llm","gemini","claude","deepseek","chatgpt","copilot","ollama","kimi","qwen","llama","mistral","midjourney","stablediffusion","openai","anthropic","model","models","grok"]),
        new("code","开发与代码",20,Set(
            ".cs",".fs",".vb",".js",".ts",".tsx",".jsx",".py",".go",".java",".kt",".kts",".rs",".cpp",".cc",".c",".h",".hpp",".csproj",".sln",".fsproj",".vbproj",".json",".yaml",".yml",".toml",".sql",".sh",".ps1",".bat",".cmd",".html",".htm",".css",".scss",".vue",".svelte",".ipynb",".lua",".rb",".php",".swift",".dart",".asm",".cmake",".gradle",".slx",".mdl",".m"),[
            "源码","代码","脚本","项目源码","接口文档","工程文件","编译","调试","算法","刷题","仓库","仿真","建模","cc switch"],["code","repo","github","gitlab","source","src","api","sdk","script","scripts","build","debug","compile","leetcode","npm","docker","dataset","project","projects","workspace","pycharm","intellij","quartus","modelsim","vivado","keil","matlab","simulink","unity","zcode","androidstudio","visualstudio"]),
        new("design","设计素材",30,Set(
            ".psd",".psb",".ai",".fig",".sketch",".xd",".cdr",".eps",".indd",".blend",".obj",".fbx",".stl",".3ds",".max",".prproj",".aep"),[
            "设计","素材","海报","封面","原型","字体","图标","插画","排版","效果图","logo","校色","颜色配置","x-rite"],["design","figma","sketch","photoshop","illustrator","mockup","icon","icons","font","typography","ui","ux","render","texture","x-rite"]),
        new("study","学习课程",40,Set(".epub",".mobi",".azw3",".djvu",".caj"),[
            "课程","学习","作业","教材","考试","复习","考研","四级","六级","课件","网课","教程","论文","题库","讲义","真题","知识点","竞赛","课设","课程设计"],["study","course","homework","lesson","tutorial","exam","thesis","lecture","quiz","mooc","coursera","udemy"]),
        new("finance","财务票据",50,Set(".ofx",".qif",".qbo"),[
            "发票","报销","账单","账本","财务","银行","收款","付款","转账","收据","税务","工资","对账","流水"],["invoice","receipt","statement","bank","tax","payroll","billing","finance"]),
        new("work","工作事务",60,Set(".eml",".msg",".ics",".vcf"),[
            "客户","报价","需求","周报","月报","日报","总结","合同","提案","方案","招标","立项","验收","排期"],["client","quote","proposal","contract","deadline","okr","kpi","agenda","minutes"]),
        new("social","社交沟通",65,Set(),[
            "微信","qq","钉钉","飞书","腾讯会议","企业微信","聊天","通讯","邮箱","邮件","会议","好友","群聊","语音通话"],["qq","wechat","weixin","wecom","dingtalk","feishu","lark","telegram","discord","whatsapp","slack","zoom","teams","mail","outlook","gmail","meeting","tencent","skype"]),
        new("apps","应用与工具",68,Set(),[
            "工具","软件","应用","助手","管家","浏览器","网盘","输入法","驱动","奶蛙","naiwa","360","wps","sangfor","easyconnect"],["tool","tools","utility","utilities","app","apps","browser","edge","chrome","firefox","wps","office","baidu","netdisk","easyconnect","sangfor","360","naiwa"]),
        new("games","游戏娱乐",70,Set(".sav",".save",".rom",".nes",".sfc",".gba",".nds",".3ds",".iso2"),[
            "游戏","存档","角色","模组","攻略","原神","崩坏","星穹","我的世界","开黑","加速器"],["steam","epic","game","games","gamer","minecraft","mod","mods","save","savegame","xbox","playstation","legion"]),
        new("docs","文档资料",80,Set(
            ".pdf",".doc",".docx",".rtf",".odt",".txt",".md",".markdown",".wps",".pages",".texi",".tex"),[
            "文档","资料","说明","手册","指南","报告","笔记","简历","通知","政策","规范","纪要","备忘","阅读"],["doc","docs","document","manual","guide","report","note","notes","resume","readme","paper","article","ebook"]),
        new("sheets","表格数据",90,Set(".xls",".xlsx",".xlsm",".csv",".ods",".numbers",".tsv"),[
            "表格","数据","统计","报表","预算","清单","名单","台账","明细","汇总"],["sheet","sheets","spreadsheet","excel","data","dataset","budget","list","inventory","tracker"]),
        new("slides","演示汇报",100,Set(".ppt",".pptx",".pptm",".odp",".key"),[
            "演示","汇报","演讲","分享","培训","答辩","宣讲"],["ppt","slides","slide","presentation","keynote","pitch","deck"]),
        new("images","图片照片",110,Set(
            ".jpg",".jpeg",".png",".gif",".bmp",".webp",".svg",".heic",".heif",".tif",".tiff",".raw",".cr2",".nef",".arw",".ico"),[
            "截图","照片","图片","壁纸","相册","图像","扫描件","表情包"],["screenshot","screenshots","photo","photos","image","images","wallpaper","picture","pictures","snipaste","screencap"]),
        new("media","音视频",120,Set(".srt",".ass",".vtt"),[
            "视频","录音","音乐","音频","剪辑","电影","歌曲","短片","直播回放","剪映","录屏","qq音乐"],["movie","video","videos","audio","music","song","songs","recording","clip","podcast","vlog","film"]),
        new("install","安装与压缩包",130,Set(),[
            "安装","压缩","备份","驱动","补丁","绿色版","便携版","固件","镜像","破解","crack"],["setup","installer","install","backup","driver","patch","portable","archive","firmware","crack"]),
        new("shortcuts","快捷方式",140,Set(".lnk",".url",".appref-ms"),[
            "快捷方式"],["shortcut","shortcuts"]),
        new("other","其他",999,Set(),[],[])
    ];

    private static Bucket ById(string id) => Buckets.First(bucket => bucket.Id == id);

    private static readonly SplitRule[] SplitRules =
    [
        new("docs",item => ExtensionIn(item,".pdf") ? "docs-pdf" : null,"docs-pdf","PDF 文档",201),
        new("docs",item => ExtensionIn(item,".doc",".docx",".rtf",".odt",".wps",".pages") ? "docs-word" : null,"docs-word","Word 文档",202),
        new("docs",item => ExtensionIn(item,".txt",".md",".markdown") ? "docs-notes" : null,"docs-notes","笔记与文本",203),
        new("docs",item => ExtensionIn(item,".epub",".mobi",".azw3",".djvu",".caj") ? "docs-books" : null,"docs-books","电子书",204),
        new("images",item => IsScreenshot(item) ? "images-shots" : null,"images-shots","截图录屏",211),
        new("images",item => IsScreenshot(item) ? null : "images-photos","images-photos","图片照片",212),
        new("media",item => ExtensionIn(item,".mp3",".wav",".flac",".aac",".m4a",".ogg",".wma",".opus") ? "media-audio" : null,"media-audio","音频",221),
        new("media",item => ExtensionIn(item,".mp4",".mkv",".mov",".avi",".wmv",".webm",".m4v",".flv") ? "media-video" : null,"media-video","视频",222),
        new("install",item => ExtensionIn(item,".zip",".7z",".rar",".tar",".gz",".bz2",".xz",".zst",".iso",".img") ? "install-archives" : null,"install-archives","压缩包",231),
        new("install",item => ExtensionIn(item,".exe",".msi",".msix",".appx",".appxbundle",".msixbundle") ? "install-setup" : null,"install-setup","安装程序",232),
        new("code",item => ExtensionIn(item,".html",".htm",".css",".scss",".js",".jsx",".ts",".tsx",".vue",".svelte") ? "code-web" : null,"code-web","前端页面",241),
        new("code",item => ExtensionIn(item,".py",".ps1",".sh",".bat",".cmd",".lua",".rb",".php") ? "code-scripts" : null,"code-scripts","脚本自动化",242),
        new("design",item => ExtensionIn(item,".psd",".psb",".ai",".fig",".sketch",".xd",".cdr",".indd",".prproj",".aep") ? "design-source" : null,"design-source","设计工程",251),
        new("sheets",item => ExtensionIn(item,".csv",".tsv") ? "sheets-csv" : null,"sheets-csv","CSV 数据",261)
    ];

    public static NaiWaSmartClassification Classify(
        IReadOnlyList<DesktopOrganizationFileSnapshot> items)
    {
        var byPath = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        var buckets = new Dictionary<string, List<DesktopOrganizationFileSnapshot>>(StringComparer.Ordinal);
        foreach (var item in items)
        {
            string id = BucketFor(item);
            if (!buckets.TryGetValue(id,out var list)) buckets[id] = list = [];
            list.Add(item);
        }

        ExpandToMinimum(buckets);
        MergeToMaximum(buckets);

        var ordered = buckets
            .Where(pair => pair.Value.Count > 0)
            .OrderBy(pair => pair.Key == "other" ? 1 : 0)
            .ThenByDescending(pair => pair.Value.Count)
            .ThenBy(pair => OrderOf(pair.Key))
            .ToList();
        var categories = new List<AiOrganizeCategory>(ordered.Count);
        var counts = new Dictionary<string, int>(StringComparer.Ordinal);
        foreach (var (id, list) in ordered)
        {
            string name = NameOf(id);
            categories.Add(new AiOrganizeCategory(id,name));
            counts[id] = list.Count;
            foreach (var item in list) byPath[item.SourcePath] = id;
        }

        return new NaiWaSmartClassification(categories,byPath,counts);
    }

    private static void ExpandToMinimum(
        Dictionary<string, List<DesktopOrganizationFileSnapshot>> buckets)
    {
        int guard = 0;
        while (PopulatedCount(buckets) < MinimumCategories && guard++ < 16)
        {
            bool split = false;
            foreach (var rule in SplitRules)
            {
                if (!buckets.TryGetValue(rule.SourceId,out var source) || source.Count < 2) continue;
                var matched = source.Where(item => rule.Match(item) is not null).ToList();
                if (matched.Count == 0 || matched.Count == source.Count) continue;
                var remaining = source.Except(matched).ToList();
                if (remaining.Count == 0) continue;
                buckets[rule.SourceId] = remaining;
                if (!buckets.TryGetValue(rule.TargetId,out var targetList))
                    buckets[rule.TargetId] = targetList = [];
                targetList.AddRange(matched);
                split = true;
                break;
            }
            if (!split) break;
        }
    }

    private static readonly string[] SpecificTopicOrder = ["ai","study","design","finance","work","media","games","code"];
    private static readonly string[] FormatOrder = ["docs","sheets","slides","images","install","shortcuts"];

    private static readonly Dictionary<string, string> MergeTarget = new(StringComparer.Ordinal)
    {
        ["ai"] = "code",
        ["design"] = "images",
        ["study"] = "docs",
        ["finance"] = "work",
        ["games"] = "media",
        ["slides"] = "docs",
        ["media"] = "images",
        ["shortcuts"] = "apps",
        ["apps"] = "other",
        ["social"] = "work",
        ["code-web"] = "code",
        ["code-scripts"] = "code",
        ["docs-pdf"] = "docs",
        ["docs-word"] = "docs",
        ["docs-notes"] = "docs",
        ["docs-books"] = "docs",
        ["images-shots"] = "images",
        ["images-photos"] = "images",
        ["media-audio"] = "media",
        ["media-video"] = "media",
        ["install-archives"] = "install",
        ["install-setup"] = "install",
        ["design-source"] = "design",
        ["sheets-csv"] = "sheets"
    };

    private static void MergeToMaximum(
        Dictionary<string, List<DesktopOrganizationFileSnapshot>> buckets)
    {
        while (PopulatedCount(buckets) > MaximumCategories)
        {
            // Fold the smallest bucket into its nearest topic instead of
            // dumping meaningful groups ("AI", "财务", "学习") into "其他".
            var smallest = buckets
                .Where(pair => pair.Value.Count > 0 && pair.Key != "other")
                .OrderBy(pair => pair.Value.Count)
                .ThenBy(pair => OrderOf(pair.Key))
                .First();
            string targetId = MergeTarget.TryGetValue(smallest.Key,out string? related) &&
                related != smallest.Key ? related : "other";
            if (!buckets.TryGetValue(targetId,out var target) || targetId == smallest.Key)
            {
                // The related bucket may already have been folded away (e.g.
                // media into images). Fall back to the largest surviving
                // bucket so every iteration strictly reduces the count.
                targetId = buckets
                    .Where(pair => pair.Key != smallest.Key && pair.Value.Count > 0)
                    .OrderByDescending(pair => pair.Value.Count)
                    .Select(pair => pair.Key)
                    .FirstOrDefault() ?? "other";
                if (!buckets.TryGetValue(targetId,out target))
                    buckets[targetId] = target = [];
            }
            target.AddRange(smallest.Value);
            buckets.Remove(smallest.Key);
        }
    }

    private static int PopulatedCount(
        Dictionary<string, List<DesktopOrganizationFileSnapshot>> buckets) =>
        buckets.Count(pair => pair.Value.Count > 0);

    private static string BucketFor(DesktopOrganizationFileSnapshot item)
    {
        string name = Path.GetFileNameWithoutExtension(item.Name).ToLowerInvariant();
        var tokens = Tokenize(name);

        // Specific purposes first: "发票.pdf" is more useful as 财务 than PDF.
        foreach (string id in SpecificTopicOrder)
            if (MatchesTopic(ById(id),name,tokens)) return id;

        // Strong file formats outrank broad app words: a 奶蛙 video is media,
        // not 应用与工具, and a 奶蛙 archive is a package.
        string extension = item.IsDirectory
            ? string.Empty
            : DesktopOrganizationClassifier.NormalizeExtension(item.Extension);
        if (!item.IsDirectory)
        {
            if (ArchiveExtensions.Contains(extension) || InstallerExtensions.Contains(extension))
                return "install";
            if (AudioExtensions.Contains(extension) || VideoExtensions.Contains(extension))
                return "media";
            foreach (string id in new[] { "images","docs","sheets","slides","code" })
                if (ById(id).Extensions.Contains(extension)) return id;
        }

        // Purpose-named apps and communication tools.
        if (MatchesTopic(ById("apps"),name,tokens)) return "apps";
        if (MatchesTopic(ById("social"),name,tokens)) return "social";

        foreach (string id in FormatOrder)
        {
            Bucket bucket = ById(id);
            if (MatchesTopic(bucket,name,tokens) ||
                (!item.IsDirectory && bucket.Extensions.Contains(extension)))
                return id;
        }

        return "other";
    }

    private static bool MatchesTopic(Bucket bucket,string name,HashSet<string> tokens) =>
        bucket.NameKeywords.Any(keyword => name.Contains(keyword,StringComparison.OrdinalIgnoreCase)) ||
        bucket.TokenKeywords.Any(keyword => tokens.Contains(keyword));

    private static HashSet<string> Tokenize(string value)
    {
        var tokens = new HashSet<string>(StringComparer.Ordinal);
        int start = -1;
        for (int i = 0; i <= value.Length; i++)
        {
            bool alphaNumeric = i < value.Length && char.IsLetterOrDigit(value[i]);
            if (alphaNumeric)
            {
                if (start < 0) start = i;
            }
            else if (start >= 0)
            {
                tokens.Add(value[start..i]);
                start = -1;
            }
        }

        return tokens;
    }

    private static bool IsScreenshot(DesktopOrganizationFileSnapshot item)
    {
        string name = Path.GetFileNameWithoutExtension(item.Name).ToLowerInvariant();
        return name.Contains("截图") || name.Contains("屏幕快照") || name.Contains("录屏") ||
            name.Contains("screenshot") || name.Contains("snipaste") ||
            name.Contains("screen capture") || name.Contains("screencap") ||
            name.Contains("qq截图") || name.Contains("微信截图");
    }

    private static bool ExtensionIn(DesktopOrganizationFileSnapshot item,params string[] extensions) =>
        extensions.Contains(DesktopOrganizationClassifier.NormalizeExtension(item.Extension),StringComparer.OrdinalIgnoreCase);

    private static string NameOf(string id) =>
        id switch
        {
            "docs-pdf" => "PDF 文档",
            "docs-word" => "Word 文档",
            "docs-notes" => "笔记与文本",
            "docs-books" => "电子书",
            "images-shots" => "截图录屏",
            "images-photos" => "图片照片",
            "media-audio" => "音频",
            "media-video" => "视频",
            "install-archives" => "压缩包",
            "install-setup" => "安装程序",
            "code-web" => "前端页面",
            "code-scripts" => "脚本自动化",
            "design-source" => "设计工程",
            "sheets-csv" => "CSV 数据",
            _ => ById(id).Name
        };

    private static int OrderOf(string id) =>
        id switch
        {
            "docs-pdf" => 201,            "docs-word" => 202,            "docs-notes" => 203,            "docs-books" => 204,
            "images-shots" => 211,            "images-photos" => 212,            "media-audio" => 221,            "media-video" => 222,
            "install-archives" => 231,            "install-setup" => 232,            "code-web" => 241,            "code-scripts" => 242,
            "design-source" => 251,            "sheets-csv" => 261,
            _ => ById(id).Order
        };

    private static HashSet<string> Set(params string[] values) =>
        new(values,StringComparer.OrdinalIgnoreCase);
}
