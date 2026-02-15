# Shandong Tender Scraper

采集山东省政府采购网的招标公告信息

## Usage

```
shandong-tender [keyword]
```

## Arguments

- `keyword` - 搜索关键词（可选，默认为"软件"）

## Examples

```bash
# 搜索软件相关招标
shandong-tender 软件

# 搜索信息化相关招标
shandong-tender 信息化

# 使用默认关键词
shandong-tender
```

## Output

返回采集到的招标信息列表，包括：
- 标题
- 发布日期
- 详情链接
- 预算金额（如有）
- 联系人（如有）
