import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { API, isMobile, showError, showSuccess } from '../../helpers';
import { renderQuota, renderQuotaWithPrompt, getQuotaPerUnit } from '../../helpers/render';
import Title from '@douyinfe/semi-ui/lib/es/typography/title';
import {
  Button,
  Divider,
  Input,
  Modal,
  Select,
  SideSheet,
  Space,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

const EditUser = (props) => {
  const userId = props.editingUser.id;
  const [loading, setLoading] = useState(true);
  const [addQuotaModalOpen, setIsModalOpen] = useState(false);
  const [addQuotaLocal, setAddQuotaLocal] = useState('');
  const [useCurrencyInput, setUseCurrencyInput] = useState(() => {
    // 根据系统设置决定默认输入模式
    const displayInCurrency = localStorage.getItem('display_in_currency');
    return displayInCurrency === 'true';
  });
  const [inputs, setInputs] = useState({
    username: '',
    display_name: '',
    password: '',
    github_id: '',
    wechat_id: '',
    email: '',
    quota: 0,
    group: 'default',
  });
  const [groupOptions, setGroupOptions] = useState([]);
  const {
    username,
    display_name,
    password,
    github_id,
    wechat_id,
    telegram_id,
    email,
    quota,
    group,
  } = inputs;
  const handleInputChange = (name, value) => {
    if (name === 'quota' && useCurrencyInput) {
      // 如果使用货币输入模式，将输入的金额转换为原始额度
      const quotaPerUnit = getQuotaPerUnit();
      const rawQuota = Math.round(parseFloat(value || 0) * quotaPerUnit);
      setInputs((inputs) => ({ ...inputs, [name]: rawQuota }));
    } else {
      setInputs((inputs) => ({ ...inputs, [name]: value }));
    }
  };

  // 获取显示用的额度值（根据输入模式）
  const getDisplayQuota = () => {
    if (useCurrencyInput) {
      const quotaPerUnit = getQuotaPerUnit();
      return (quota / quotaPerUnit).toFixed(2);
    }
    return quota;
  };

  // 切换输入模式
  const toggleInputMode = () => {
    setUseCurrencyInput(!useCurrencyInput);
  };
  const fetchGroups = async () => {
    try {
      let res = await API.get(`/api/group/`);
      const { success, message, data } = res.data;
      if (success) {
        // 如果不是超级管理员，只显示包含用户名的分组
        let filteredGroups = data;
        const currentUser = JSON.parse(localStorage.getItem('user'));
        if (currentUser?.role < 100) {
          filteredGroups = data.filter(group => 
            group.includes(currentUser.username)
          );
        }
        setGroupOptions(
          filteredGroups.map((group) => ({
            label: group,
            value: group,
          }))
        );
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    }
  };
  const navigate = useNavigate();
  const handleCancel = () => {
    props.handleClose();
  };
  const loadUser = async () => {
    setLoading(true);
    let res = undefined;
    if (userId) {
      res = await API.get(`/api/user/${userId}`);
    } else {
      res = await API.get(`/api/user/self`);
    }
    const { success, message, data } = res.data;
    if (success) {
      data.password = '';
      setInputs(data);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  useEffect(() => {
    loadUser().then();
    if (userId) {
      fetchGroups().then();
    }
  }, [props.editingUser.id]);

  const submit = async () => {
    setLoading(true);
    let res = undefined;
    if (userId) {
      let data = { ...inputs, id: parseInt(userId) };
      if (typeof data.quota === 'string') {
        data.quota = parseInt(data.quota);
      }
      res = await API.put(`/api/user/`, data);
    } else {
      res = await API.put(`/api/user/self`, inputs);
    }
    const { success, message } = res.data;
    if (success) {
      showSuccess('用户信息更新成功！');
      props.refresh();
      props.handleClose();
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const addLocalQuota = () => {
    let newQuota;
    if (useCurrencyInput) {
      // 如果使用货币输入模式，将输入的金额转换为原始额度
      const quotaPerUnit = getQuotaPerUnit();
      const addAmount = parseFloat(addQuotaLocal || 0);
      const addRawQuota = Math.round(addAmount * quotaPerUnit);
      newQuota = quota + addRawQuota;
    } else {
      newQuota = parseInt(quota) + parseInt(addQuotaLocal || 0);
    }
    setInputs((inputs) => ({ ...inputs, quota: newQuota }));
  };

  const openAddQuotaModal = () => {
    setAddQuotaLocal('0');
    setIsModalOpen(true);
  };

  const { t } = useTranslation();

  return (
    <>
      <SideSheet
        placement={'right'}
        title={<Title level={3}>{t('编辑用户')}</Title>}
        headerStyle={{ borderBottom: '1px solid var(--semi-color-border)' }}
        bodyStyle={{ borderBottom: '1px solid var(--semi-color-border)' }}
        visible={props.visible}
        footer={
          <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <Space>
              <Button theme='solid' size={'large'} onClick={submit}>
                {t('提交')}
              </Button>
              <Button
                theme='solid'
                size={'large'}
                type={'tertiary'}
                onClick={handleCancel}
              >
                {t('取消')}
              </Button>
            </Space>
          </div>
        }
        closeIcon={null}
        onCancel={() => handleCancel()}
        width={isMobile() ? '100%' : 600}
      >
        <Spin spinning={loading}>
          <div style={{ marginTop: 20 }}>
            <Typography.Text>{t('用户名')}</Typography.Text>
          </div>
          <Input
            label={t('用户名')}
            name='username'
            placeholder={t('请输入新的用户名')}
            onChange={(value) => handleInputChange('username', value)}
            value={username}
            autoComplete='new-password'
          />
          <div style={{ marginTop: 20 }}>
            <Typography.Text>{t('密码')}</Typography.Text>
          </div>
          <Input
            label={t('密码')}
            name='password'
            type={'password'}
            placeholder={t('请输入新的密码，最短 8 位')}
            onChange={(value) => handleInputChange('password', value)}
            value={password}
            autoComplete='new-password'
          />
          <div style={{ marginTop: 20 }}>
            <Typography.Text>{t('显示名称')}</Typography.Text>
          </div>
          <Input
            label={t('显示名称')}
            name='display_name'
            placeholder={t('请输入新的显示名称')}
            onChange={(value) => handleInputChange('display_name', value)}
            value={display_name}
            autoComplete='new-password'
          />
          {userId && (
            <>
              <div style={{ marginTop: 20 }}>
                <Typography.Text>{t('分组')}</Typography.Text>
              </div>
              <Select
                placeholder={t('请选择分组')}
                name='group'
                fluid
                search
                selection
                allowAdditions
                additionLabel={t('请在系统设置页面编辑分组倍率以添加新的分组：')}
                onChange={(value) => handleInputChange('group', value)}
                value={inputs.group}
                autoComplete='new-password'
                optionList={groupOptions}
              />
              <div style={{ marginTop: 20 }}>
                <Typography.Text>{`${t('剩余额度')}${renderQuotaWithPrompt(quota)}`}</Typography.Text>
              </div>
              <Space>
                <Input
                  name='quota'
                  placeholder={useCurrencyInput ? t('请输入金额（美元）') : t('请输入新的剩余额度')}
                  onChange={(value) => handleInputChange('quota', value)}
                  value={getDisplayQuota()}
                  type={'number'}
                  step={useCurrencyInput ? '0.01' : '1'}
                  autoComplete='new-password'
                />
                <Button onClick={openAddQuotaModal}>{t('添加额度')}</Button>
                <Button onClick={toggleInputMode} type="tertiary">
                  {useCurrencyInput ? t('切换到原始额度') : t('切换到货币输入')}
                </Button>
              </Space>
              <div style={{ marginTop: 10 }}>
                <Typography.Text type="secondary" size="small">
                  {useCurrencyInput 
                    ? t('当前模式：货币输入（美元）') 
                    : t('当前模式：原始额度输入')
                  }
                </Typography.Text>
              </div>
            </>
          )}
          <Divider style={{ marginTop: 20 }}>{t('以下信息不可修改')}</Divider>
          <div style={{ marginTop: 20 }}>
            <Typography.Text>{t('已绑定的 GitHub 账户')}</Typography.Text>
          </div>
          <Input
            name='github_id'
            value={github_id}
            autoComplete='new-password'
            placeholder={t('此项只读，需要用户通过个人设置页面的相关绑定按钮进行绑定，不可直接修改')}
            readonly
          />
          <div style={{ marginTop: 20 }}>
            <Typography.Text>{t('已绑定的微信账户')}</Typography.Text>
          </div>
          <Input
            name='wechat_id'
            value={wechat_id}
            autoComplete='new-password'
            placeholder={t('此项只读，需要用户通过个人设置页面的相关绑定按钮进行绑定，不可直接修改')}
            readonly
          />
          <div style={{ marginTop: 20 }}>
            <Typography.Text>{t('已绑定的邮箱账户')}</Typography.Text>
          </div>
          <Input
            name='email'
            value={email}
            autoComplete='new-password'
            placeholder={t('此项只读，需要用户通过个人设置页面的相关绑定按钮进行绑定，不可直接修改')}
            readonly
          />
          <div style={{ marginTop: 20 }}>
            <Typography.Text>{t('已绑定的Telegram账户')}</Typography.Text>
          </div>
          <Input
            name='telegram_id'
            value={telegram_id}
            autoComplete='new-password'
            placeholder={t('此项只读，需要用户通过个人设置页面的相关绑定按钮进行绑定，不可直接修改')}
            readonly
          />
        </Spin>
      </SideSheet>
      <Modal
        centered={true}
        visible={addQuotaModalOpen}
        onOk={() => {
          addLocalQuota();
          setIsModalOpen(false);
        }}
        onCancel={() => setIsModalOpen(false)}
        closable={null}
      >
        <div style={{ marginTop: 20 }}>
          <Typography.Text>{t('当前额度')}：{renderQuota(quota)}</Typography.Text>
        </div>
        <div style={{ marginTop: 10 }}>
          <Typography.Text>{t('添加额度')}：</Typography.Text>
        </div>
        <Input
          name='addQuotaLocal'
          placeholder={useCurrencyInput ? t('需要添加的金额（美元，支持负数）') : t('需要添加的额度（支持负数）')}
          onChange={(value) => {
            setAddQuotaLocal(value);
          }}
          value={addQuotaLocal}
          type={'number'}
          step={useCurrencyInput ? '0.01' : '1'}
          autoComplete='new-password'
        />
        <div style={{ marginTop: 10 }}>
          <Typography.Text>{t('新额度')}：{renderQuota(quota + (useCurrencyInput ? Math.round(parseFloat(addQuotaLocal || 0) * getQuotaPerUnit()) : parseInt(addQuotaLocal || 0)))}</Typography.Text>
        </div>
        <div style={{ marginTop: 10 }}>
          <Typography.Text type="secondary" size="small">
            {useCurrencyInput 
              ? t('当前模式：货币输入（美元）') 
              : t('当前模式：原始额度输入')
            }
          </Typography.Text>
        </div>
      </Modal>
    </>
  );
};

export default EditUser;
