

const Topic = {
    NodeRmv : "topic_node_remove",
    NodeHistoryClean : "topic_history_clean",   // 清空 cmd list
    NodeUpdateParm: "topic_node_update_parm",

    NodeGraphClick: "topic_node_graph_click",  // 在editor的编辑器视图中点击节点
    NodeEditorClick: "topic_node_editor_click",    //在editor的edit框中点击节点类型

    ThemeChange: "theme_change",

    BotsUpdate: "topic_bots_update",   // 机器人列表需要更新

    PrefabUpdateAll: "topic_prefab_update_all",

    FileSave : "topic_file_save",
    FileLoadDraw: "topic_file_load_draw",
    FileLoadRedraw : "topic_file_load_graph",

    ReportSelect : "topic_report_select",

}

export default Topic;